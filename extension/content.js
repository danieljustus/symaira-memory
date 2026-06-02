// content.js - Injects the Symmemory trigger button into ChatGPT, Claude, and Perplexity
console.log('⚡ Symaira Memory Context Injector loaded.');

// Listen for DOM changes to dynamically inject button near textareas
const observer = new MutationObserver((mutations) => {
  injectTriggerButtons();
});
observer.observe(document.body, { childList: true, subtree: true });
injectTriggerButtons();

function injectTriggerButtons() {
  // Common selectors for chat inputs
  // ChatGPT: #prompt-textarea
  // Claude: div.ProseMirror or textarea
  // Perplexity: textarea
  const textareas = document.querySelectorAll('textarea, div[contenteditable="true"]');
  
  textareas.forEach((area) => {
    // Avoid double injections
    if (area.dataset.symmemoryInjected === 'true') return;
    
    // Skip small search boxes or unrelated text areas
    if (area.offsetWidth < 200 || area.offsetHeight < 25) return;

    // Find immediate parent or containing form to position our absolute button
    const parent = area.parentElement;
    if (!parent) return;

    area.dataset.symmemoryInjected = 'true';
    
    // Create button container
    const btn = document.createElement('button');
    btn.className = 'symmemory-inject-btn';
    btn.innerHTML = '⚡';
    btn.title = 'Inject Symaira Memory Context';
    btn.type = 'button';

    // Premium CSS styling for the floating button
    Object.assign(btn.style, {
      position: 'absolute',
      right: '12px',
      bottom: '12px',
      zIndex: '1000',
      width: '32px',
      height: '32px',
      borderRadius: '50%',
      border: '1px solid #A2EEEF',
      background: 'rgba(30, 30, 46, 0.9)',
      color: '#A2EEEF',
      fontSize: '16px',
      cursor: 'pointer',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      transition: 'all 0.2s ease-in-out',
      boxShadow: '0 0 10px rgba(162, 238, 239, 0.2)',
      outline: 'none'
    });

    // Hover animations
    btn.addEventListener('mouseenter', () => {
      btn.style.transform = 'scale(1.1)';
      btn.style.boxShadow = '0 0 15px rgba(162, 238, 239, 0.6)';
      btn.style.background = '#1E1E2E';
    });

    btn.addEventListener('mouseleave', () => {
      btn.style.transform = 'scale(1.0)';
      btn.style.boxShadow = '0 0 10px rgba(162, 238, 239, 0.2)';
      btn.style.background = 'rgba(30, 30, 46, 0.9)';
    });

    // Click handler to pull local context and prefix it!
    btn.addEventListener('click', async (e) => {
      e.preventDefault();
      e.stopPropagation();

      const userText = getAreaValue(area).trim();
      const searchQuery = userText || "general"; // use user input or default
      
      btn.innerHTML = '⏳';
      btn.style.borderColor = '#F9E2AF';
      btn.style.color = '#F9E2AF';

      try {
        const response = await chrome.runtime.sendMessage({
          action: 'search_memory',
          query: searchQuery,
          limit: 3
        });

        if (response && response.success && response.memories && response.memories.length > 0) {
          // Format memory block
          const facts = response.memories.map(m => `- ${m.content}`).join('\n');
          const contextBlock = `\n\n[Symaira Memory: \n${facts}\n]\n\n`;
          
          injectTextIntoArea(area, contextBlock);
          showToast('⚡ Memory Context Injected!');
        } else {
          showToast('❌ No matching memories found.');
        }
      } catch (err) {
        showToast('❌ Daemon inactive (Run: symmemory serve -p 8787)');
      } finally {
        btn.innerHTML = '⚡';
        btn.style.borderColor = '#A2EEEF';
        btn.style.color = '#A2EEEF';
      }
    });

    // Handle relative positioning of parent if needed
    const parentStyle = window.getComputedStyle(parent);
    if (parentStyle.position === 'static') {
      parent.style.position = 'relative';
    }
    
    parent.appendChild(btn);
  });
}

function getAreaValue(area) {
  if (area.tagName === 'TEXTAREA') {
    return area.value;
  }
  return area.innerText;
}

function injectTextIntoArea(area, text) {
  if (area.tagName === 'TEXTAREA') {
    const start = area.selectionStart;
    const end = area.selectionEnd;
    const oldVal = area.value;
    area.value = oldVal.substring(0, start) + text + oldVal.substring(end);
    area.focus();
    
    // Dispatch input event so React/Vue sites detect change!
    area.dispatchEvent(new Event('input', { bubbles: true }));
  } else {
    // Contenteditable divs (like Claude Web)
    area.focus();
    // In contenteditable, we can append or inject at cursor
    const selection = window.getSelection();
    if (selection.rangeCount > 0) {
      const range = selection.getRangeAt(0);
      range.deleteContents();
      const textNode = document.createTextNode(text);
      range.insertNode(textNode);
      range.collapse(false);
    } else {
      area.innerText += text;
    }
    area.dispatchEvent(new Event('input', { bubbles: true }));
  }
}

function showToast(message) {
  const toast = document.createElement('div');
  toast.className = 'symmemory-toast';
  toast.innerText = message;
  
  Object.assign(toast.style, {
    position: 'fixed',
    top: '20px',
    right: '20px',
    zIndex: '10000',
    padding: '12px 24px',
    borderRadius: '8px',
    background: '#1E1E2E',
    border: '1px solid #A2EEEF',
    color: '#CDD6F4',
    fontFamily: 'Inter, system-ui, sans-serif',
    fontWeight: 'bold',
    boxShadow: '0 4px 15px rgba(0, 0, 0, 0.4)',
    transform: 'translateY(-20px)',
    opacity: '0',
    transition: 'all 0.3s ease-out'
  });
  
  document.body.appendChild(toast);
  
  // Animate in
  setTimeout(() => {
    toast.style.transform = 'translateY(0)';
    toast.style.opacity = '1';
  }, 10);

  // Fade out and remove
  setTimeout(() => {
    toast.style.transform = 'translateY(-20px)';
    toast.style.opacity = '0';
    setTimeout(() => toast.remove(), 300);
  }, 2200);
}
