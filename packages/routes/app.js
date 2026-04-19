import { Router } from 'express';

/**
 * Creates the unified fzt-frontend tree routes.
 *
 * Every tree is an entity with a flat string id (e.g. `nelson-bookmarks`,
 * `nelson-menu`, `google`). No namespace, no ACL scoping — any
 * authenticated caller can read/write any tree. Identity-based scoping
 * is enforced client-side by choosing which ids to fetch/save.
 *
 * Trees are stored as append-only versioned docs in Cosmos. Each save
 * bumps `version` by 1. GET returns the latest.
 *
 * Doc schema:
 *   {
 *     id: `tree_<treeId>_v<N>`,   // unique per version
 *     userId: '<treeId>',         // /userId partition value — one partition per tree
 *     treeId: '<treeId>',
 *     type: 'tree',
 *     version: N,
 *     tree: ...,                  // array or object
 *     updatedAt: ISO string
 *   }
 *
 * The container's partition key path is `/userId` (legacy name from
 * its pre-tree era). Each tree lives in its own partition keyed by its
 * id. `userId` and `treeId` always hold the same value on new docs.
 *
 * Refs within a tree use the shape `{ ref: "<treeId>" }`. The GET
 * handler recursively expands them (cycle-guarded, depth-limited),
 * tagging resolved nodes with `_ref`/`_refVersion` so PUT can strip
 * them back to pointer form. Cross-tree writes are NOT performed — if
 * a caller wants to edit a referenced tree, they PUT that tree's id
 * directly (see fzt-frontend#4).
 *
 * @param {{
 *   requireAuth: Function,
 *   container: import('@azure/cosmos').Container,  // HomepageDB.fzt-frontend-data
 * }} opts
 */
export function createFztFrontendRoutes({ requireAuth, container }) {
  const router = Router();

  const MAX_REF_DEPTH = 10;

  function versionDocId(treeId, version) {
    return `tree_${treeId}_v${version}`;
  }

  async function getLatestTree(treeId) {
    const { resources } = await container.items.query({
      query: `SELECT TOP 1 * FROM c
              WHERE c.type = 'tree' AND c.treeId = @id
              ORDER BY c.version DESC`,
      parameters: [{ name: '@id', value: treeId }],
    }, { partitionKey: treeId }).fetchAll();
    return resources[0] || null;
  }

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
        const target = await getLatestTree(item.ref);
        if (!target) {
          out.push({ ...item, _refError: true });
          continue;
        }
        const body = target.tree;
        const resolvedNode = {
          _ref: item.ref,
          _refVersion: target.version,
        };
        if (Array.isArray(body)) {
          const nextVisited = new Set(visited); nextVisited.add(item.ref);
          resolvedNode.children = await resolveRefs(body, nextVisited);
        } else if (body && typeof body === 'object') {
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

  // GET /tree/:id — read latest version + resolve refs
  router.get('/tree/:id', requireAuth, async (req, res) => {
    try {
      const { id } = req.params;
      const latest = await getLatestTree(id);
      if (!latest) {
        // Never-saved tree — return empty at v0 so consumers don't branch on 404.
        // A subsequent PUT with baseVersion=0 creates v1.
        return res.json({ id, tree: [], version: 0, updatedAt: null });
      }
      const resolved = await resolveRefs(latest.tree);
      res.json({
        id,
        tree: resolved,
        version: latest.version,
        updatedAt: latest.updatedAt,
      });
    } catch (error) {
      console.error('Error fetching tree:', error);
      res.status(500).json({ error: 'Failed to fetch tree', message: error.message });
    }
  });

  // PUT /tree/:id — create new version; body: { tree, baseVersion }
  router.put('/tree/:id', requireAuth, async (req, res) => {
    try {
      const { id } = req.params;
      const { tree, baseVersion } = req.body;
      if (tree === undefined || tree === null) {
        return res.status(400).json({ error: 'Request body must contain a tree field' });
      }

      const latest = await getLatestTree(id);
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
        id: versionDocId(id, newVersion),
        userId: id,
        treeId: id,
        type: 'tree',
        version: newVersion,
        tree: stripped,
        updatedAt: now,
      });

      const resolved = await resolveRefs(stripped);
      res.json({
        id,
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
