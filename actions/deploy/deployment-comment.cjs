'use strict';

const titleCollator = new Intl.Collator('en', {
  numeric: true,
  sensitivity: 'base',
});

function normalizeDeploymentResults(results) {
  if (!Array.isArray(results)) {
    return [];
  }

  return results
    .filter((result) => result && typeof result.uid === 'string' && result.uid.trim() !== '')
    .map((result) => {
      const uid = result.uid.trim();
      const title =
        typeof result.title === 'string' && result.title.trim() !== '' ? result.title.trim() : uid;
      return { uid, title };
    })
    .sort(
      (first, second) =>
        titleCollator.compare(first.title, second.title) ||
        titleCollator.compare(first.uid, second.uid)
    );
}

function parseDeploymentResults(details, uidList = '') {
  let parsed = [];
  try {
    parsed = JSON.parse(details || '[]');
  } catch {
    parsed = [];
  }

  if (Array.isArray(parsed) && parsed.length > 0) {
    return normalizeDeploymentResults(parsed);
  }

  const fallback = String(uidList || '')
    .split(/\s+/)
    .filter(Boolean)
    .map((uid) => ({ uid, title: uid }));

  return normalizeDeploymentResults(fallback);
}

function renderAlertList(alerts, grafanaInstance, includeLinks) {
  const baseUrl = String(grafanaInstance || '')
    .replaceAll('"', '')
    .replace(/\/+$/, '');

  return normalizeDeploymentResults(alerts)
    .map(({ uid, title }) => {
      if (includeLinks && baseUrl) {
        return `- [${title}](${baseUrl}/alerting/grafana/${encodeURIComponent(uid)}/view)`;
      }
      return `- ${title}`;
    })
    .join('\n');
}

function buildDeploymentComment({ created = [], updated = [], deleted = [], grafanaInstance = '' }) {
  const createdAlerts = normalizeDeploymentResults(created);
  const updatedAlerts = normalizeDeploymentResults(updated);
  const deletedAlerts = normalizeDeploymentResults(deleted);

  return `
## Sigma Rule Deployment Status

| Created | Updated | Deleted |
| --- | --- | --- |
| ${createdAlerts.length} | ${updatedAlerts.length} | ${deletedAlerts.length} |

### Created

${renderAlertList(createdAlerts, grafanaInstance, true)}

### Updated

${renderAlertList(updatedAlerts, grafanaInstance, true)}

### Deleted

${renderAlertList(deletedAlerts, grafanaInstance, false)}
`;
}

module.exports = {
  buildDeploymentComment,
  normalizeDeploymentResults,
  parseDeploymentResults,
};
