// Service Worker to handle local REST port proxying
const LOCAL_API_URL = 'http://127.0.0.1:8787';

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  // Wrap in async immediately to satisfy Chrome's runtime handler
  (async () => {
    try {
      if (message.action === 'check_status') {
        const status = await checkDaemonStatus();
        sendResponse({ success: true, status });
      } else if (message.action === 'search_memory') {
        const memories = await queryLocalMemories(message.query, message.scope, message.limit);
        sendResponse({ success: true, memories });
      } else if (message.action === 'set_memory') {
        const result = await saveLocalMemory(message.content, message.scope, message.metadata);
        sendResponse({ success: true, result });
      } else {
        sendResponse({ success: false, error: 'Unknown action' });
      }
    } catch (err) {
      console.error('API connection error:', err);
      sendResponse({ success: false, error: err.message });
    }
  })();
  
  return true; // Keep the message channel open for asynchronous responses!
});

async function checkDaemonStatus() {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 1000); // Fast 1s timeout
  
  try {
    const res = await fetch(`${LOCAL_API_URL}/api/status`, { signal: controller.signal });
    clearTimeout(timeoutId);
    if (!res.ok) return 'inactive';
    const data = await res.json();
    return data.status === 'healthy' ? 'active' : 'inactive';
  } catch (err) {
    clearTimeout(timeoutId);
    return 'inactive';
  }
}

async function queryLocalMemories(query, scope, limit = 5) {
  const res = await fetch(`${LOCAL_API_URL}/api/search`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, scope, limit })
  });
  
  if (!res.ok) {
    throw new Error(`HTTP search failure: ${res.status}`);
  }
  return await res.json();
}

async function saveLocalMemory(content, scope, metadata = {}) {
  const res = await fetch(`${LOCAL_API_URL}/api/set`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content, scope, metadata })
  });

  if (!res.ok) {
    throw new Error(`HTTP save failure: ${res.status}`);
  }
  return await res.json();
}
