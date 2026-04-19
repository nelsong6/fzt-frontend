import { AppConfigurationClient } from '@azure/app-configuration';
import { SecretClient } from '@azure/keyvault-secrets';
import { DefaultAzureCredential } from '@azure/identity';

export async function fetchConfig() {
  const appConfigEndpoint = process.env.AZURE_APP_CONFIG_ENDPOINT;
  const keyVaultUrl = process.env.KEY_VAULT_URL;
  if (!appConfigEndpoint) throw new Error('AZURE_APP_CONFIG_ENDPOINT unset');
  if (!keyVaultUrl) throw new Error('KEY_VAULT_URL unset');

  const credential = new DefaultAzureCredential();
  const appConfig = new AppConfigurationClient(appConfigEndpoint, credential);
  const kv = new SecretClient(keyVaultUrl, credential);

  const cosmosEndpoint = await appConfig.getConfigurationSetting({ key: 'cosmos_db_endpoint' });
  const jwtSigningSecret = (await kv.getSecret('api-jwt-signing-secret')).value;

  return {
    cosmosDbEndpoint: cosmosEndpoint.value,
    jwtSigningSecret,
  };
}
