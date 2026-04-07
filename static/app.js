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

    // CIDR field: auto-populate, IP presence warning
    const cidrField = document.getElementById('cidr_allow');
    if (cidrField) {
        const clientIP = cidrField.dataset.clientIp;
        let cidrWarning = document.createElement('div');
        cidrWarning.className = 'cidr-warning';
        cidrWarning.hidden = true;
        cidrField.parentNode.insertBefore(cidrWarning, cidrField.nextSibling);

        // Auto-fill on first focus if empty
        let firstFocus = true;
        cidrField.addEventListener('focus', function() {
            if (firstFocus && this.value.trim() === '' && clientIP) {
                this.value = clientIP + '/32';
                const len = this.value.length;
                this.setSelectionRange(len, len);
            }
            firstFocus = false;
        });

        function checkCIDRWarning() {
            const val = cidrField.value.trim();
            if (val === '') {
                cidrWarning.hidden = true;
                return;
            }
            if (!clientIP) { cidrWarning.hidden = true; return; }

            // Check if client IP is in any of the listed CIDRs
            const cidrs = val.split('\n').map(s => s.trim()).filter(Boolean);
            const ipInList = cidrs.some(cidr => ipMatchesCIDR(clientIP, cidr));

            if (!ipInList) {
                cidrWarning.innerHTML = '<span class="warning-icon">&#9888;</span> Your IP <strong>' +
                    esc(clientIP) + '</strong> is not in this list. On page reload, this link will be invisible to you. ' +
                    '<a href="#" class="add-ip-link">Add my IP</a>';
                cidrWarning.hidden = false;
                cidrWarning.querySelector('.add-ip-link').addEventListener('click', function(e) {
                    e.preventDefault();
                    const current = cidrField.value.trim();
                    cidrField.value = current + (current ? '\n' : '') + clientIP + '/32';
                    checkCIDRWarning();
                    cidrField.focus();
                    const len = cidrField.value.length;
                    cidrField.setSelectionRange(len, len);
                });
            } else {
                cidrWarning.hidden = true;
            }
        }

        // Simple IP-in-CIDR check (supports /32 exactly and basic prefix matching)
        function ipMatchesCIDR(ip, cidr) {
            const parts = cidr.split('/');
            const cidrIP = parts[0];
            const bits = parts.length > 1 ? parseInt(parts[1], 10) : 32;
            if (isNaN(bits) || bits < 0 || bits > 32) return false;

            const ipNum = ipToNum(ip);
            const cidrNum = ipToNum(cidrIP);
            if (ipNum === null || cidrNum === null) return false;

            const mask = bits === 0 ? 0 : (~0 << (32 - bits)) >>> 0;
            return (ipNum & mask) === (cidrNum & mask);
        }

        function ipToNum(ip) {
            const parts = ip.split('.');
            if (parts.length !== 4) return null;
            let num = 0;
            for (let i = 0; i < 4; i++) {
                const v = parseInt(parts[i], 10);
                if (isNaN(v) || v < 0 || v > 255) return null;
                num = (num * 256) + v;
            }
            return num >>> 0;
        }

        cidrField.addEventListener('input', checkCIDRWarning);
        // Check on page load too (for existing links)
        checkCIDRWarning();
    }

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
