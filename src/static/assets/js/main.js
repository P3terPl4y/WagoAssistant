/**
 * Wago Global Utilities
 */

// --- Toast Notification System ---
const Toast = {
    init: function() {
        if (!document.getElementById('toast-container')) {
            const container = document.createElement('div');
            container.id = 'toast-container';
            document.body.appendChild(container);
        }
    },
    show: function(message, type = 'info', duration = 4000) {
        this.init();
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        
        let iconClass = 'fas fa-info-circle';
        if (type === 'success') iconClass = 'fas fa-check-circle';
        if (type === 'error') iconClass = 'fas fa-exclamation-circle';
        if (type === 'warning') iconClass = 'fas fa-exclamation-triangle';
        
        toast.innerHTML = `
            <i class="${iconClass}"></i>
            <span>${message}</span>
        `;
        
        container.appendChild(toast);
        
        // Trigger reflow for animation
        void toast.offsetWidth;
        toast.classList.add('show');
        
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => {
                if (container.contains(toast)) {
                    container.removeChild(toast);
                }
            }, 400); // Wait for transition to finish
        }, duration);
    }
};

// Override standard alert to use Toast if needed (Optional, but helps with lazy refactors)
// window.alert = function(msg) { Toast.show(msg, 'info'); };

// --- Dark Mode Toggle ---
const ThemeManager = {
    init: function() {
        const savedTheme = localStorage.getItem('theme');
        if (savedTheme === 'dark') {
            document.documentElement.setAttribute('data-theme', 'dark');
        }
    },
    toggle: function() {
        const currentTheme = document.documentElement.getAttribute('data-theme');
        if (currentTheme === 'dark') {
            document.documentElement.removeAttribute('data-theme');
            localStorage.setItem('theme', 'light');
        } else {
            document.documentElement.setAttribute('data-theme', 'dark');
            localStorage.setItem('theme', 'dark');
        }
    }
};

// Initialize on load
document.addEventListener('DOMContentLoaded', () => {
    Toast.init();
    ThemeManager.init();
    Modal.init();
});

// --- Custom Modal System ---
const Modal = {
    init: function() {
        if (!document.getElementById('modal-overlay')) {
            const overlay = document.createElement('div');
            overlay.id = 'modal-overlay';
            overlay.innerHTML = `
                <div class="modal-box glass-card">
                    <h3 id="modal-title"></h3>
                    <p id="modal-message"></p>
                    <input type="text" id="modal-input" class="form-control" style="display:none; margin: 16px 0;" />
                    <div class="modal-actions" style="display:flex; justify-content:flex-end; gap:12px; margin-top:20px;">
                        <button id="modal-cancel" class="btn btn-secondary" style="color:var(--color-text-main);">Cancelar</button>
                        <button id="modal-confirm" class="btn btn-solid-green">Confirmar</button>
                    </div>
                </div>
            `;
            
            // Basic styles for modal
            const style = document.createElement('style');
            style.textContent = `
                #modal-overlay {
                    position: fixed; top: 0; left: 0; width: 100%; height: 100%;
                    background: rgba(0,0,0,0.5); backdrop-filter: blur(4px);
                    z-index: 10001; display: flex; align-items: center; justify-content: center;
                    opacity: 0; pointer-events: none; transition: opacity 0.3s;
                }
                #modal-overlay.show { opacity: 1; pointer-events: auto; }
                .modal-box { padding: 32px; width: 90%; max-width: 400px; transform: translateY(-20px); transition: transform 0.3s; background: var(--color-surface); }
                #modal-overlay.show .modal-box { transform: translateY(0); }
                .modal-box h3 { margin-bottom: 12px; font-family: var(--font-headings); }
                .modal-box p { color: var(--color-text-muted); font-size: 0.95rem; }
            `;
            document.head.appendChild(style);
            document.body.appendChild(overlay);
        }
    },
    confirm: function(title, message) {
        return new Promise((resolve) => {
            this.init();
            const overlay = document.getElementById('modal-overlay');
            document.getElementById('modal-title').textContent = title;
            document.getElementById('modal-message').textContent = message;
            document.getElementById('modal-input').style.display = 'none';
            
            const btnConfirm = document.getElementById('modal-confirm');
            const btnCancel = document.getElementById('modal-cancel');
            
            const cleanup = () => {
                overlay.classList.remove('show');
                btnConfirm.replaceWith(btnConfirm.cloneNode(true));
                btnCancel.replaceWith(btnCancel.cloneNode(true));
            };
            
            btnConfirm.onclick = () => { cleanup(); resolve(true); };
            btnCancel.onclick = () => { cleanup(); resolve(false); };
            
            overlay.classList.add('show');
        });
    },
    prompt: function(title, message, placeholder = '') {
        return new Promise((resolve) => {
            this.init();
            const overlay = document.getElementById('modal-overlay');
            document.getElementById('modal-title').textContent = title;
            document.getElementById('modal-message').textContent = message;
            
            const input = document.getElementById('modal-input');
            input.style.display = 'block';
            input.value = '';
            input.placeholder = placeholder;
            
            const btnConfirm = document.getElementById('modal-confirm');
            const btnCancel = document.getElementById('modal-cancel');
            
            const cleanup = () => {
                overlay.classList.remove('show');
                btnConfirm.replaceWith(btnConfirm.cloneNode(true));
                btnCancel.replaceWith(btnCancel.cloneNode(true));
            };
            
            btnConfirm.onclick = () => { cleanup(); resolve(input.value); };
            btnCancel.onclick = () => { cleanup(); resolve(null); };
            
            overlay.classList.add('show');
            setTimeout(() => input.focus(), 100);
        });
    }
};
