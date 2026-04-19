import { Router } from 'express';

/**
 * Creates the unified fzt-frontend tree routes.
 *
 * Data model: every tree is an entity with an ID of the form `<namespace>/<name>`.
 * Namespaces: the caller's JWT `sub` for personal trees (read/write), or `shared`
 * for cross-identity trees (read for everyone, write scoped — see below).
 *
 * Trees are stored as append-only versioned docs in Cosmos. Each save bumps
 * `version` by 1. GET returns the latest.
 *
 * Doc schema:
 *   {
 *     id: `tree_<ns>_<name>_v<N>`,  // unique per version
 *     userId: '<partition>',        // /userId partition value (ns or "shared:<name>")
 *     namespace: '<ns>',
 *     name: '<name>',
 *     type: 'tree',
 *     version: N,
 *     tree: [...],                  // the actual tree body
 *     updatedAt: ISO string
 *   }
 *
 * The container's partition key is `/userId` (legacy name from its pre-tree
 * bookmarks era). Personal trees partition on the namespace itself;
 * shared trees partition on `shared:<name>` so each shared tree gets its
 * own partition rather than crowding a single `shared` one.
 *
 * Refs within a tree use the shape `{ ref: "<ns>/<name>" }`. The GET handler
 * recursively expands them (cycle-guarded, depth-limited), tagging resolved
 * nodes with `_ref`/`_refVersion` so PUT can decompose them back. Cross-ref
 * writes are NOT performed — decomposition only strips metadata. If a user
 * wants to edit a referenced tree, they call PUT on that tree's ID directly.
 *
 * @param {{
 *   requireAuth: Function,
 *   container: import('@azure/cosmos').Container,  // HomepageDB.fzt-frontend-data
 * }} opts
 */
export function createFztFrontendRoutes({ requireAuth, container }) {
  const router = Router();

  const MAX_REF_DEPTH = 10;

  // The fzt-frontend-data container is partitioned on /userId — a legacy
  // name from when it only held bookmarks. Trees keep using it: personal
  // namespaces collapse to `userId = namespace`, shared trees get one
  // partition per name (`userId = "shared:<name>"`) to avoid a hot
  // single "shared" partition.
  function partitionFor(namespace, name) {
    return namespace === 'shared' ? `shared:${name}` : namespace;
  }

  function versionDocId(namespace, name, version) {
    return `tree_${namespace}_${name}_v${version}`;
  }

  async function getLatestTree(namespace, name) {
    const pk = partitionFor(namespace, name);
    const { resources } = await container.items.query({
      query: `SELECT TOP 1 * FROM c
              WHERE c.type = 'tree' AND c.namespace = @ns AND c.name = @name
              ORDER BY c.version DESC`,
      parameters: [
        { name: '@ns', value: namespace },
        { name: '@name', value: name },
      ],
    }, { partitionKey: pk }).fetchAll();
    return resources[0] || null;
  }

  // Recursively expand { ref: "<ns>/<name>" } nodes. Nodes whose only
  // non-metadata key is `ref` get replaced with the resolved tree's content,
  // tagged `_ref` / `_refVersion`. Unknown refs render as `_refError: true`.
  async function resolveRefs(items, visited = new Set()) {
    if (!Array.isArray(items)) return items;
    const out = [];
    for (const item of items) {
      if (!item || typeof item !== 'object') {
        out.push(item);
        continue;
      }
      const keysWithoutError = Object.keys(item).filter(k => k !== '_refError');
      if (typeof item.ref === 'string' && keysWithoutError.length === 1) {
        if (visited.has(item.ref) || visited.size >= MAX_REF_DEPTH) {
          out.push({ ...item, _refError: true });
          continue;
        }
        const [ns, nm] = item.ref.split('/', 2);
        if (!ns || !nm) {
          out.push({ ...item, _refError: true });
          continue;
        }
        const target = await getLatestTree(ns, nm);
        if (!target) {
          out.push({ ...item, _refError: true });
          continue;
        }
        // Inline the target's tree body as the node's children, carry name
        // + description from the target if present (tree root may have them).
        const body = target.tree;
        const resolvedNode = {
          _ref: item.ref,
          _refVersion: target.version,
        };
        if (Array.isArray(body)) {
          const nextVisited = new Set(visited); nextVisited.add(item.ref);
          resolvedNode.children = await resolveRefs(body, nextVisited);
        } else if (body && typeof body === 'object') {
          // Root is a single-object tree (like today's shared-google blob shape).
          Object.assign(resolvedNode, body);
          if (Array.isArray(body.children)) {
            const nextVisited = new Set(visited); nextVisited.add(item.ref);
            resolvedNode.children = await resolveRefs(body.children, nextVisited);
          }
        }
        out.push(resolvedNode);
      } else if (Array.isArray(item.children) && item.children.length > 0) {
        out.push({ ...item, children: await resolveRefs(item.children, visited) });
      } else {
        out.push(item);
      }
    }
    return out;
  }

  // Strip resolved refs back to pointer form on PUT. Does NOT write to the
  // referenced trees — ref edits are read-only from the enclosing tree's
  // perspective. If the caller wants to edit a ref target, they PUT that
  // tree's ID directly.
  function stripRefs(items) {
    if (!Array.isArray(items)) return items;
    return items.map(item => {
      if (!item || typeof item !== 'object') return item;
      if (typeof item._ref === 'string') {
        return { ref: item._ref };
      }
      if (Array.isArray(item.children) && item.children.length > 0) {
        return { ...item, children: stripRefs(item.children) };
      }
      return item;
    });
  }

  // Authorization: caller can read their own namespace and `shared`.
  // Caller can write their own namespace; writes to `shared` require the
  // caller to be the owner (for now, any authenticated user — we can tighten
  // this later when/if we add granular ACLs on shared refs).
  function canRead(caller, ns) { return ns === caller || ns === 'shared'; }
  function canWrite(caller, ns) { return ns === caller || ns === 'shared'; }

  // `me` is a client-side convenience — substitutes the caller's sub so the
  // browser frontend can address its own trees without a whoami round-trip.
  function resolveNs(rawNs, caller) { return rawNs === 'me' ? caller : rawNs; }

  // GET /tree/:ns/:name — read latest version + resolve refs
  router.get('/tree/:ns/:name', requireAuth, async (req, res) => {
    try {
      const { name } = req.params;
      const caller = req.user.sub;
      const ns = resolveNs(req.params.ns, caller);
      if (!canRead(caller, ns)) {
        return res.status(403).json({ error: 'Forbidden' });
      }
      const latest = await getLatestTree(ns, name);
      if (!latest) {
        // Fresh identity — no tree stored yet. Return an empty tree at
        // version 0 so consumers can treat "never saved" as the zero state
        // without branching on 404s. A subsequent PUT with baseVersion=0
        // creates v1.
        return res.json({
          id: `${ns}/${name}`,
          tree: [],
          version: 0,
          updatedAt: null,
        });
      }
      const resolved = await resolveRefs(latest.tree);
      res.json({
        id: `${ns}/${name}`,
        tree: resolved,
        version: latest.version,
        updatedAt: latest.updatedAt,
      });
    } catch (error) {
      console.error('Error fetching tree:', error);
      res.status(500).json({ error: 'Failed to fetch tree', message: error.message });
    }
  });

  // PUT /tree/:ns/:name — create new version; body: { tree, baseVersion }
  router.put('/tree/:ns/:name', requireAuth, async (req, res) => {
    try {
      const { name } = req.params;
      const caller = req.user.sub;
      const ns = resolveNs(req.params.ns, caller);
      if (!canWrite(caller, ns)) {
        return res.status(403).json({ error: 'Forbidden' });
      }
      const { tree, baseVersion } = req.body;
      if (tree === undefined || tree === null) {
        return res.status(400).json({ error: 'Request body must contain a tree field' });
      }

      const latest = await getLatestTree(ns, name);
      const latestVersion = latest?.version || 0;
      if (baseVersion !== undefined && baseVersion !== null && baseVersion !== latestVersion) {
        return res.status(409).json({
          error: 'Conflict detected',
          message: 'Tree has been modified elsewhere.',
          currentTree: latest ? await resolveRefs(latest.tree) : [],
          currentVersion: latestVersion,
        });
      }

      const stripped = stripRefs(tree);
      const newVersion = latestVersion + 1;
      const now = new Date().toISOString();

      await container.items.create({
        id: versionDocId(ns, name, newVersion),
        userId: partitionFor(ns, name),
        namespace: ns,
        name,
        type: 'tree',
        version: newVersion,
        tree: stripped,
        updatedAt: now,
      });

      const resolved = await resolveRefs(stripped);
      res.json({
        id: `${ns}/${name}`,
        tree: resolved,
        version: newVersion,
        updatedAt: now,
      });
    } catch (error) {
      console.error('Error saving tree:', error);
      res.status(500).json({ error: 'Failed to save tree', message: error.message });
    }
  });

  return router;
}
