// popup.js - Controls the Symaira Memory popup dashboard interface
document.addEventListener('DOMContentLoaded', async () => {
  await updatePopupState();
});

async function updatePopupState() {
  const statusBadge = document.getElementById('status-badge');
  const factCountEl = document.getElementById('fact-count');
  const listEl = document.getElementById('memory-list');

  try {
    // Send message to background service worker to check status
    const statusResponse = await chrome.runtime.sendMessage({ action: 'check_status' });
    
    if (statusResponse && statusResponse.status === 'active') {
      // Daemon is active!
      statusBadge.innerText = 'DAEMON ACTIVE';
      statusBadge.className = 'badge status-active';

      // Load all saved memories to display
      const memoryResponse = await chrome.runtime.sendMessage({
        action: 'search_memory',
        query: '', // empty query defaults to standard listing
        limit: 10
      });

      if (memoryResponse && memoryResponse.success && memoryResponse.memories) {
        const memories = memoryResponse.memories;
        factCountEl.innerText = memories.length;

        if (memories.length === 0) {
          listEl.innerHTML = '<div class="loading">No memories saved yet. Use the chat triggers or command line to add facts!</div>';
        } else {
          listEl.innerHTML = '';
          memories.forEach((mem) => {
            const item = document.createElement('div');
            item.className = 'memory-item';
            item.innerHTML = `
              <span class="scope-tag">${mem.Scope}</span>
              <div class="text">${escapeHtml(mem.Content)}</div>
            `;
            listEl.appendChild(item);
          });
        }
      } else {
        listEl.innerHTML = '<div class="loading">Failed to fetch memory elements.</div>';
      }
    } else {
      // Daemon is offline
      setOfflineState(statusBadge, factCountEl, listEl);
    }
  } catch (err) {
    console.error('Error connecting to background worker:', err);
    setOfflineState(statusBadge, factCountEl, listEl);
  }
}

function setOfflineState(statusBadge, factCountEl, listEl) {
  statusBadge.innerText = 'DAEMON OFFLINE';
  statusBadge.className = 'badge status-inactive';
  factCountEl.innerText = '--';
  listEl.innerHTML = '<div class="loading" style="color: #f38ba8;">Please run <code style="font-size:10px;">symmemory serve -p 8787</code> on your local computer to connect!</div>';
}

function escapeHtml(text) {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
