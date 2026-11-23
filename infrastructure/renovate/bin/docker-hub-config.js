// Docker Hub's tags API requires a JWT; registry Basic auth is not enough.
// Renovate loads this function once per job, obtaining a fresh Hub session.
module.exports = async function dockerHubConfig() {
  const username = process.env.DOCKER_HUB_USERNAME;
  const password = process.env.DOCKER_HUB_TOKEN;
  if (!username || !password) {
    throw new Error('Missing DOCKER_HUB_USERNAME or DOCKER_HUB_TOKEN');
  }

  let response;
  try {
    response = await fetch('https://hub.docker.com/v2/auth/token', {
      method: 'POST',
      redirect: 'error',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ identifier: username, secret: password }),
      signal: AbortSignal.timeout(30000),
    });
  } catch {
    throw new Error('Docker Hub authentication request failed');
  }
  if (!response.ok) {
    throw new Error(`Docker Hub authentication failed (HTTP ${response.status})`);
  }
  let session;
  try {
    session = await response.json();
  } catch {
    throw new Error('Docker Hub returned an invalid authentication response');
  }
  if (typeof session.access_token !== 'string' || !session.access_token) {
    throw new Error('Docker Hub authentication response has no access token');
  }

  return {
    hostRules: [
      { hostType: 'docker', matchHost: 'docker.io', username, password },
      {
        hostType: 'docker',
        matchHost: 'https://hub.docker.com/',
        token: session.access_token,
        authType: 'Bearer',
      },
    ],
  };
};
