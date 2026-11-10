// Keel Real-Time Reactive Dashboard Engine

const ICONS = {
  rocket: `<svg class="icon-svg" viewBox="0 0 24 24"><path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"></path><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"></path><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0"></path><path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"></path></svg>`,
  clock: `<svg class="icon-svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>`,
  alert: `<svg class="icon-svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line></svg>`,
  check: `<svg class="icon-svg" viewBox="0 0 24 24"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>`,
  user: `<svg class="icon-svg" viewBox="0 0 24 24"><path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"></path><circle cx="12" cy="7" r="4"></circle></svg>`,
  link: `<svg class="icon-svg" viewBox="0 0 24 24"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>`,
  package: `<svg class="icon-svg" viewBox="0 0 24 24"><line x1="16.5" y1="9.4" x2="7.5" y2="4.21"></line><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"></path><polyline points="3.27 6.96 12 12.01 20.73 6.96"></polyline><line x1="12" y1="22.08" x2="12" y2="12"></line></svg>`
};

let lastPayloadHash = '';

function renderProject(data) {
  const currentHash = JSON.stringify(data);
  if (currentHash === lastPayloadHash) {
    return;
  }
  lastPayloadHash = currentHash;

  document.getElementById('project-name').textContent = data.manifest.name;
  document.getElementById('project-phase').textContent = 'Phase: ' + (data.state.currentPhase || 'Active');
  document.getElementById('project-progress').textContent = data.state.progress || 'In progress';
  
  // Health
  const healthEl = document.getElementById('project-health');
  healthEl.textContent = data.state.health ? data.state.health.replace('_', ' ') : 'on track';
  healthEl.className = 'badge ' + (data.state.health || 'on_track');

  // Stats from Evaluation
  const evalData = data.evaluation || { ready: [], inProgress: [], blocked: [], completed: [] };
  const total = data.tasks ? data.tasks.length : 0;
  const readyCount = evalData.ready ? evalData.ready.length : 0;
  const inProgressCount = evalData.inProgress ? evalData.inProgress.length : 0;
  const blockedCount = evalData.blocked ? evalData.blocked.length : 0;
  const completedCount = evalData.completed ? evalData.completed.length : 0;

  document.getElementById('stat-ready').textContent = readyCount;
  document.getElementById('stat-active').textContent = inProgressCount;
  document.getElementById('stat-blocked').textContent = blockedCount;
  document.getElementById('stat-completed').textContent = `${completedCount} / ${total}`;

  // Update Column Header Titles with SVG Icons
  document.getElementById('title-ready').innerHTML = `${ICONS.rocket} Ready`;
  document.getElementById('title-inprogress').innerHTML = `${ICONS.clock} In Progress`;
  document.getElementById('title-blocked').innerHTML = `${ICONS.alert} Blocked`;
  document.getElementById('title-completed').innerHTML = `${ICONS.check} Completed`;

  document.getElementById('count-ready').textContent = readyCount;
  document.getElementById('count-inprogress').textContent = inProgressCount;
  document.getElementById('count-blocked').textContent = blockedCount;
  document.getElementById('count-completed').textContent = completedCount;

  // Goals Banner
  const goalEl = document.getElementById('goal-content');
  if (data.goals && data.goals.length > 0) {
    const activeGoal = data.goals.find(g => g.id === data.state.currentGoalId) || data.goals.find(g => g.status === 'active') || data.goals[0];
    let html = `<div style="margin-bottom:8px; font-size:15px;"><strong>${escapeHtml(activeGoal.title)}</strong></div>`;
    if (activeGoal.outcome) {
      html += `<p style="font-size:13px; color:var(--text-secondary); margin-bottom:10px;">${escapeHtml(activeGoal.outcome)}</p>`;
    }
    if (activeGoal.doneWhen && activeGoal.doneWhen.length > 0) {
      html += `<div style="font-size:12px; font-weight:600; color:var(--text-muted); margin-bottom:4px;">DONE WHEN:</div><ul style="padding-left:18px; font-size:13px;">`;
      activeGoal.doneWhen.forEach(cond => {
        html += `<li>${escapeHtml(cond)}</li>`;
      });
      html += `</ul>`;
    }
    goalEl.innerHTML = html;
  } else {
    goalEl.innerHTML = `<p style="color:var(--text-muted); font-size:13px;">No goals defined. Run 'keel goal create' to set an outcome.</p>`;
  }

  // Kanban Columns
  renderColumnCards('cards-ready', evalData.ready || [], 'ready', true);
  renderColumnCards('cards-inprogress', evalData.inProgress || [], 'in_progress', false);
  renderColumnCards('cards-blocked', evalData.blocked || [], 'blocked', true);
  renderColumnCards('cards-completed', evalData.completed || [], 'completed', false);

  // Worklog
  const worklogEl = document.getElementById('latest-worklog');
  if (data.latestWorklog) {
    let nextHtml = '';
    if (data.latestWorklog.nextSteps && data.latestWorklog.nextSteps.length > 0) {
      nextHtml = `<div style="font-size:12px; font-weight:600; color:var(--text-muted); margin-top:12px;">NEXT ACTIONS FOR AGENT:</div><ul class="next-list">`;
      data.latestWorklog.nextSteps.forEach(step => {
        nextHtml += `<li>→ ${escapeHtml(step)}</li>`;
      });
      nextHtml += `</ul>`;
    }

    worklogEl.innerHTML = `
      <div class="worklog-box">
        <div class="worklog-meta">Session by <strong>${escapeHtml(data.latestWorklog.harness)}</strong> • ${new Date(data.latestWorklog.createdAt).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})}</div>
        <div class="worklog-summary">${escapeHtml(data.latestWorklog.summary)}</div>
        ${nextHtml}
      </div>
    `;
  } else {
    worklogEl.innerHTML = `<p style="color:var(--text-muted); font-size:13px;">No worklogs recorded yet. Agents leave notes with 'keel worklog'.</p>`;
  }
}

function renderColumnCards(containerId, items, categoryStatus, isWrapped) {
  const container = document.getElementById(containerId);
  if (!container) return;

  if (items.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted); font-size:12px; text-align:center; padding:20px 0;">Empty</div>`;
    return;
  }

  let html = '';
  items.forEach(entry => {
    const task = isWrapped ? entry.task : entry;
    const blockers = isWrapped ? entry.blockers : null;
    html += renderTaskCard(task, categoryStatus, blockers);
  });
  container.innerHTML = html;
}

function renderTaskCard(task, categoryStatus, blockers) {
  const prio = task.priority || 'medium';
  const prioClass = 'prio-' + prio.toLowerCase();

  let metaParts = [];
  if (task.assignee) {
    metaParts.push(`${ICONS.user} ${escapeHtml(task.assignee)}`);
  }
  if (task.dependsOn && task.dependsOn.length > 0) {
    metaParts.push(`${ICONS.link} ${escapeHtml(task.dependsOn.join(', '))}`);
  }

  let blockerHtml = '';
  if (blockers && blockers.length > 0) {
    blockerHtml = `<div class="task-blocker">${ICONS.alert} Blocked by: ${escapeHtml(blockers.join(', '))}</div>`;
  } else if (task.blockedReason) {
    blockerHtml = `<div class="task-blocker">${ICONS.alert} Blocked: ${escapeHtml(task.blockedReason)}</div>`;
  }

  let resultHtml = '';
  if (task.result) {
    resultHtml = `<div class="task-result">${ICONS.check} Deliverable: ${escapeHtml(task.result)}</div>`;
  } else if (task.deliverables && task.deliverables.length > 0) {
    resultHtml = `<div class="task-result">${ICONS.package} Deliverables: ${escapeHtml(task.deliverables.join(', '))}</div>`;
  }

  return `
    <div class="task-item">
      <div style="flex: 1; padding-right: 8px;">
        <div class="task-id">
          <span>${escapeHtml(task.id)}</span>
          <span class="prio-badge ${prioClass}">${escapeHtml(prio)}</span>
        </div>
        <div class="task-title">${escapeHtml(task.title)}</div>
        ${task.description ? `<div class="task-desc">${escapeHtml(task.description)}</div>` : ''}
        ${metaParts.length > 0 ? `<div class="task-meta">${metaParts.join(' • ')}</div>` : ''}
        ${blockerHtml}
        ${resultHtml}
      </div>
    </div>
  `;
}

function escapeHtml(text) {
  if (!text) return '';
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// Initial fetch
fetch('/api/project')
  .then(res => res.json())
  .then(data => renderProject(data))
  .catch(err => console.error('Initial load error:', err));

// Connect to Server-Sent Events for real-time reactivity
if (window.EventSource) {
  const events = new EventSource('/api/events');
  events.addEventListener('update', function(e) {
    try {
      const data = JSON.parse(e.data);
      renderProject(data);
    } catch(err) {
      console.error('Error parsing SSE update:', err);
    }
  });

  const dot = document.querySelector('.pulse-dot');
  events.onerror = function() {
    document.getElementById('connection-status').textContent = 'Reconnecting...';
    if (dot) dot.classList.add('disconnected');
  };
  events.onopen = function() {
    document.getElementById('connection-status').textContent = 'Live Connected';
    if (dot) dot.classList.remove('disconnected');
  };
}
