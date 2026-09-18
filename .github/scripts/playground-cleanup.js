'use strict';

const sessionMarker = '<!-- reeve:playground-session:v1 -->';
const expiryPattern = /<!-- reeve:playground-expires-at:([^\s]+) -->/;
const runPattern = /<!-- reeve:playground-run:(\d+) -->/;
const legacyLifetimeMs = 6 * 60 * 60 * 1000;

function playgroundCapacity(runs, maximum = 100) {
  return runs.filter(run => run.status !== 'completed').length < maximum;
}

function sessionExpiry(pr) {
  const explicit = pr.body?.match(expiryPattern)?.[1];
  const parsed = Date.parse(explicit || '');
  if (Number.isFinite(parsed)) return parsed;
  return Date.parse(pr.created_at) + legacyLifetimeMs;
}

function ownedPlayground(pr, repository) {
  return pr.base.ref === 'master' && pr.head.repo?.full_name === repository &&
    pr.head.ref.startsWith('reeve-playground/') && pr.user?.login === 'reeve-e2e-author[bot]' &&
    pr.body?.includes(sessionMarker);
}

async function associatedRun({github, context, pr}) {
  const runID = Number(pr.body?.match(runPattern)?.[1]);
  if (Number.isSafeInteger(runID) && runID > 0) {
    try {
      const {data} = await github.rest.actions.getWorkflowRun({...context.repo, run_id: runID});
      return data;
    } catch (error) {
      if (error.status !== 404) throw error;
    }
  }

  const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
    ...context.repo, workflow_id: 'playground.yml', event: 'workflow_dispatch', per_page: 100
  });
  const prefix = `Reeve playground #${pr.number} · `;
  return runs.find(run => run.display_title?.startsWith(prefix));
}

async function cleanupPlaygrounds({github, authorToken, context, core, now = Date.now()}) {
  const repository = `${context.repo.owner}/${context.repo.repo}`;
  const authorHeaders = authorToken ? {authorization: `Bearer ${authorToken}`} : undefined;
  const pulls = await github.paginate(github.rest.pulls.list, {...context.repo, state: 'open', per_page: 100});
  const candidates = pulls.filter(pr => pr.labels.some(label => label.name === 'reeve-playground'));
  const result = {discovered: candidates.length, closed: 0, deleted: 0, active: 0, fresh: 0, skipped: 0, failed: 0};

  for (const pr of candidates) {
    try {
      if (!ownedPlayground(pr, repository)) {
        result.skipped++;
        core.warning(`Skipping unowned labelled PR #${pr.number}`);
        continue;
      }
      if (now < sessionExpiry(pr)) {
        result.fresh++;
        continue;
      }
      const run = await associatedRun({github, context, pr});
      if (run && run.status !== 'completed') {
        result.active++;
        continue;
      }
      const files = await github.paginate(github.rest.pulls.listFiles, {...context.repo, pull_number: pr.number, per_page: 100});
      if (files.length !== 1 || !/^playground\/(opentofu|terraform|pulumi)\.yaml$/.test(files[0].filename)) {
        result.skipped++;
        core.warning(`Skipping PR #${pr.number} with an unexpected diff`);
        continue;
      }
      await github.rest.pulls.update({...context.repo, pull_number: pr.number, state: 'closed', headers: authorHeaders});
      result.closed++;
      try {
        await github.rest.git.deleteRef({...context.repo, ref: `heads/${pr.head.ref}`, headers: authorHeaders});
        result.deleted++;
      } catch (error) {
        if (error.status !== 404 && error.status !== 422) throw error;
      }
    } catch (error) {
      result.failed++;
      core.error(`Cleanup failed for PR #${pr.number}: ${error.message}`);
    }
  }

  await core.summary
    .addHeading('Reeve playground cleanup')
    .addTable([
      [
        {data: 'Discovered', header: true}, {data: 'Closed', header: true},
        {data: 'Branches deleted', header: true}, {data: 'Active', header: true},
        {data: 'Fresh', header: true}, {data: 'Skipped', header: true}, {data: 'Failed', header: true}
      ],
      [
        String(result.discovered), String(result.closed), String(result.deleted),
        String(result.active), String(result.fresh), String(result.skipped), String(result.failed)
      ]
    ]).write();
  if (result.failed > 0) core.setFailed(`${result.failed} playground cleanup operation(s) failed`);
  return result;
}

module.exports = {cleanupPlaygrounds, ownedPlayground, playgroundCapacity, sessionExpiry};
