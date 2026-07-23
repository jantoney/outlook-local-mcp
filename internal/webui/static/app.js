(() => {
  const stateKey = 'outlook-mcp:open-details';

  const detailKey = (details) => {
    if (details.dataset.detailKey) return details.dataset.detailKey;
    const account = details.closest('[data-account-label]')?.dataset.accountLabel || 'page';
    return `${account}:${details.className}:${details.querySelector(':scope > summary')?.textContent.trim()}`;
  };

  const openKeys = () => {
    try { return new Set(JSON.parse(sessionStorage.getItem(stateKey) || '[]')); } catch (_) { return new Set(); }
  };

  const saveDetails = () => {
    const keys = [...document.querySelectorAll('details[open]')].map(detailKey);
    sessionStorage.setItem(stateKey, JSON.stringify(keys));
  };

  const restoreDetails = (root = document) => {
    const keys = openKeys();
    const detailsList = root.matches?.('details') ? [root, ...root.querySelectorAll('details')] : root.querySelectorAll('details');
    detailsList.forEach((details) => {
      if (keys.has(detailKey(details))) details.open = true;
    });
  };

  const bindAutosave = (root = document) => {
    root.querySelectorAll('.autosave:not([data-bound])').forEach((form) => {
      form.dataset.bound = 'true';
      let timer;
      form.addEventListener('change', () => {
        const state = form.querySelector('.save-state');
        state.textContent = 'Saving…';
        clearTimeout(timer);
        timer = setTimeout(async () => {
          try {
            const response = await fetch(form.action, { method: 'POST', body: new FormData(form), credentials: 'same-origin' });
            if (!response.ok) throw new Error(`Save failed (${response.status})`);
            saveDetails();
            window.location.assign(response.url);
          } catch (error) {
            state.textContent = error.message;
            state.classList.add('failed');
          }
        }, 180);
      });
    });
  };

  const refreshShared = async (details) => {
    const form = details.querySelector('[data-refresh-form]');
    if (!form || form.querySelector('button').disabled || details.dataset.refreshing) return;
    details.dataset.refreshing = 'true';
    const status = form.querySelector('.refresh-state');
    status.textContent = 'Refreshing…';
    try {
      saveDetails();
      const response = await fetch(form.action, { method: 'POST', body: new FormData(form), credentials: 'same-origin' });
      if (!response.ok) throw new Error(`Refresh failed (${response.status})`);
      const html = await response.text();
      const nextPage = new DOMParser().parseFromString(html, 'text/html');
      const label = details.closest('[data-account-label]').dataset.accountLabel;
      const nextCard = [...nextPage.querySelectorAll('[data-account-label]')].find((card) => card.dataset.accountLabel === label);
      const card = details.closest('[data-account-label]');
      if (!nextCard || !card) throw new Error('Updated account could not be rendered');
      card.replaceWith(nextCard);
      restoreDetails(nextCard);
      const nextShared = nextCard.querySelector('[data-refresh-on-open]');
      nextShared.dataset.skipNextRefresh = 'true';
      nextShared.open = true;
      nextShared.querySelector('.refresh-state').textContent = 'Updated just now';
      bind(nextCard);
    } catch (error) {
      status.textContent = error.message;
      details.dataset.refreshing = '';
    }
  };

  const bindDetails = (root = document) => {
    const detailsList = root.matches?.('details:not([data-details-bound])') ? [root, ...root.querySelectorAll('details:not([data-details-bound])')] : root.querySelectorAll('details:not([data-details-bound])');
    detailsList.forEach((details) => {
      details.dataset.detailsBound = 'true';
      details.addEventListener('toggle', () => {
        saveDetails();
        if (details.dataset.skipNextRefresh) {
          delete details.dataset.skipNextRefresh;
          return;
        }
        if (!details.open || !details.matches('[data-refresh-on-open]')) return;
        refreshShared(details);
      });
      if (details.open && details.matches('[data-refresh-on-open]')) {
        if (!details.dataset.skipNextRefresh) refreshShared(details);
      }
    });
  };

  const bindCopy = (root = document) => {
    root.querySelectorAll('[data-copy-value]:not([data-copy-bound])').forEach((button) => {
      button.dataset.copyBound = 'true';
      button.addEventListener('click', async () => {
        await navigator.clipboard.writeText(button.dataset.copyValue);
        const label = button.textContent;
        button.textContent = 'Copied';
        window.setTimeout(() => { button.textContent = label; }, 1200);
      });
    });
  };

  const bindRefreshForms = (root = document) => {
    root.querySelectorAll('[data-refresh-form]:not([data-submit-bound])').forEach((form) => {
      form.dataset.submitBound = 'true';
      form.addEventListener('submit', (event) => {
        event.preventDefault();
        refreshShared(form.closest('[data-refresh-on-open]'));
      });
    });
  };

  const bind = (root = document) => {
    bindAutosave(root);
    bindDetails(root);
    bindCopy(root);
    bindRefreshForms(root);
  };

  const updateAuthentication = (session) => {
    document.querySelector('#auth-state').textContent = session.state;
    document.querySelector('#auth-prompt').textContent = session.prompt || '';
    const error = document.querySelector('#auth-error');
    error.textContent = session.error || '';
    error.hidden = !session.error;
	document.querySelector('#auth-retry').hidden = !(session.user_code || session.state === 'failed');
    const link = session.auth_url || session.verification_url || '';
    const linkRow = document.querySelector('#auth-link-row');
    const codeRow = document.querySelector('#auth-code-row');
    document.querySelector('#auth-instructions').hidden = !(link || session.user_code);
    linkRow.hidden = !link;
    codeRow.hidden = !session.user_code;
    if (link) {
      document.querySelector('#auth-link').href = link;
      linkRow.querySelector('button').dataset.copyValue = link;
    }
    if (session.user_code) {
      document.querySelector('#auth-code').textContent = session.user_code;
      codeRow.querySelector('button').dataset.copyValue = session.user_code;
    }
  };

  const pollAuthentication = () => {
    const panel = document.querySelector('[data-session-id]');
    if (!panel) return;
    const poll = async () => {
      try {
        const response = await fetch(`/auth/${encodeURIComponent(panel.dataset.sessionId)}`, { credentials: 'same-origin' });
		if (response.status === 410 || response.status === 404) {
			updateAuthentication({
				state: 'failed',
				error: 'Authentication session expired. Get a new sign-in code.',
			});
			return;
		}
		if (!response.ok) throw new Error(`Authentication status failed (${response.status})`);
        const session = await response.json();
        updateAuthentication(session);
        if (session.state === 'complete') {
          saveDetails();
          window.setTimeout(() => window.location.assign('/?notice=Authentication+complete.'), 500);
          return;
        }
        if (!['failed', 'cancelled'].includes(session.state)) window.setTimeout(poll, 1000);
      } catch (_) { window.setTimeout(poll, 2000); }
    };
    window.setTimeout(poll, 600);
  };

  restoreDetails();
  bind();
  window.addEventListener('pagehide', saveDetails);
  pollAuthentication();
})();
