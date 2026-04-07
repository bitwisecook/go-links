(function() {
    'use strict';

    // Explore page: load and filter links
    const searchInput = document.getElementById('search');
    const linksContainer = document.getElementById('links');

    if (searchInput && linksContainer) {
        let allLinks = [];

        fetch('/api/links')
            .then(r => r.json())
            .then(links => {
                allLinks = links || [];
                renderLinks(allLinks);
            })
            .catch(() => {
                linksContainer.innerHTML = '<div class="no-results">Failed to load links.</div>';
            });

        searchInput.addEventListener('input', function() {
            const q = this.value.toLowerCase().trim();
            if (!q) {
                renderLinks(allLinks);
                return;
            }
            const filtered = allLinks.filter(l =>
                l.name.includes(q) ||
                (l.description && l.description.toLowerCase().includes(q)) ||
                (l.tags && l.tags.some(t => t.toLowerCase().includes(q)))
            );
            renderLinks(filtered);
        });
    }

    function renderLinks(links) {
        if (!linksContainer) return;
        if (links.length === 0) {
            linksContainer.innerHTML = '<div class="no-results">No links found.</div>';
            return;
        }
        linksContainer.innerHTML = links.map(l => {
            const tags = (l.tags || []).map(t => `<span class="link-card-tag">${esc(t)}</span>`).join('');
            const flags = [];
            if (l.has_js) flags.push('<span class="flag">JS</span>');
            if (l.restricted) flags.push('<span class="flag flag-restricted">IP</span>');
            if (l.has_args) flags.push('<span class="flag flag-args">Args</span>');

            return `<a href="/${esc(l.name)}" class="link-card">
                <div class="link-card-name">go/${esc(l.name)}</div>
                ${l.description ? `<div class="link-card-desc">${esc(l.description)}</div>` : ''}
                <div class="link-card-url">${esc(l.url)}</div>
                ${tags ? `<div class="link-card-tags">${tags}</div>` : ''}
                ${flags.length ? `<div class="link-card-flags">${flags.join('')}</div>` : ''}
            </a>`;
        }).join('');
    }

    // Admin: delete link
    window.deleteLink = function(name) {
        if (!confirm('Delete go/' + name + '?')) return;

        fetch('/admin/delete/' + encodeURIComponent(name), { method: 'POST' })
            .then(r => r.json())
            .then(data => {
                if (data.status === 'ok') {
                    const row = document.getElementById('row-' + name);
                    if (row) row.remove();
                } else {
                    alert('Error: ' + (data.error || 'Unknown error'));
                }
            })
            .catch(() => alert('Network error'));
    };

    // Edit form: async save with spinner/tick/cross
    const linkForm = document.getElementById('link-form');
    if (linkForm) {
        linkForm.addEventListener('submit', function(e) {
            e.preventDefault();

            const saveBtn = document.getElementById('save-btn');
            const btnText = saveBtn.querySelector('.btn-text');
            const btnSpinner = saveBtn.querySelector('.btn-spinner');
            const btnTick = saveBtn.querySelector('.btn-tick');
            const btnCross = saveBtn.querySelector('.btn-cross');
            const status = document.getElementById('save-status');

            // Reset state
            saveBtn.className = 'btn btn-primary saving';
            btnText.hidden = true;
            btnSpinner.hidden = false;
            btnTick.hidden = true;
            btnCross.hidden = true;
            status.hidden = true;

            const formData = new FormData(linkForm);

            fetch('/admin/save', { method: 'POST', body: formData })
                .then(r => r.json().then(data => ({ ok: r.ok, data })))
                .then(({ ok, data }) => {
                    btnSpinner.hidden = true;
                    if (ok && data.status === 'ok') {
                        saveBtn.className = 'btn btn-primary success';
                        btnTick.hidden = false;
                        status.textContent = 'Saved!';
                        status.className = 'save-status success';
                        status.hidden = false;

                        // If new link, update form to edit mode
                        const isNewField = linkForm.querySelector('[name="is_new"]');
                        if (isNewField && isNewField.value === 'true') {
                            isNewField.value = 'false';
                            const nameInput = document.getElementById('name');
                            if (nameInput) nameInput.readOnly = true;
                        }

                        setTimeout(() => {
                            saveBtn.className = 'btn btn-primary';
                            btnTick.hidden = true;
                            btnText.hidden = false;
                            status.hidden = true;
                        }, 2000);
                    } else {
                        saveBtn.className = 'btn btn-primary error';
                        btnCross.hidden = false;
                        status.textContent = data.error || 'Save failed';
                        status.className = 'save-status error';
                        status.hidden = false;

                        setTimeout(() => {
                            saveBtn.className = 'btn btn-primary';
                            btnCross.hidden = true;
                            btnText.hidden = false;
                        }, 3000);
                    }
                })
                .catch(err => {
                    btnSpinner.hidden = true;
                    saveBtn.className = 'btn btn-primary error';
                    btnCross.hidden = false;
                    status.textContent = 'Network error';
                    status.className = 'save-status error';
                    status.hidden = false;

                    setTimeout(() => {
                        saveBtn.className = 'btn btn-primary';
                        btnCross.hidden = true;
                        btnText.hidden = false;
                    }, 3000);
                });
        });
    }

    function esc(s) {
        if (!s) return '';
        const d = document.createElement('div');
        d.textContent = s;
        return d.innerHTML;
    }
})();
