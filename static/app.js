(function() {
    'use strict';

    // ===== Explore page: load and filter links =====
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
                l.name.toLowerCase().includes(q) ||
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

    // ===== Admin list: delete link =====
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

    // ===== CIDR helpers =====
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

    function isValidCIDR(s) {
        const parts = s.split('/');
        if (parts.length !== 2) return false;
        if (ipToNum(parts[0]) === null) return false;
        const bits = parseInt(parts[1], 10);
        return !isNaN(bits) && bits >= 0 && bits <= 32;
    }

    // ===== Edit form: autosave with per-row indicators =====
    const linkForm = document.getElementById('link-form');
    if (linkForm) {
        const DEBOUNCE_MS = 750;
        const RESERVED = ['admin', 'api', 'static', 'opensearch.xml', 'favicon.ico'];
        let debounceTimer = null;
        let saving = false;
        let pendingSave = false;
        let dirtyFields = new Set(); // tracks which fields changed since last save

        // Snapshot of saved values to detect changes
        let savedValues = {};
        linkForm.querySelectorAll('input:not([type=hidden]), textarea').forEach(el => {
            if (el.name) savedValues[el.name] = el.value;
        });

        const statusEl = document.getElementById('autosave-status');
        const statusIdle = statusEl.querySelector('.autosave-idle');
        const statusSaving = statusEl.querySelector('.autosave-saving');
        const statusSaved = statusEl.querySelector('.autosave-saved');
        const statusError = statusEl.querySelector('.autosave-error');
        const statusErrorMsg = statusEl.querySelector('.autosave-error-msg');

        let isNew = linkForm.dataset.isNew === 'true';

        function showGlobalStatus(which) {
            statusIdle.hidden = which !== 'idle';
            statusSaving.hidden = which !== 'saving';
            statusSaved.hidden = which !== 'saved';
            statusError.hidden = which !== 'error';
        }
        // On load: show saved if editing existing, idle if new
        showGlobalStatus(isNew ? 'idle' : 'saved');

        // --- Per-row state management ---
        function setRowState(fieldName, state) {
            const row = linkForm.querySelector('.form-row[data-field="' + fieldName + '"]');
            if (!row) return;
            row.classList.remove('row-saved', 'row-editing', 'row-saving', 'row-error');
            if (state) row.classList.add('row-' + state);
        }

        function setAllRowState(state) {
            linkForm.querySelectorAll('.form-row').forEach(row => {
                row.classList.remove('row-saved', 'row-editing', 'row-saving', 'row-error');
                if (state) row.classList.add('row-' + state);
            });
        }

        // --- Field validation ---
        const nameField = document.getElementById('name');
        const urlField = document.getElementById('url');
        const cidrField = document.getElementById('cidr_allow');

        function validateName() {
            if (!nameField || nameField.readOnly) return true;
            const v = nameField.value.trim();
            if (v === '') { setInvalid(nameField, 'Name is required'); return false; }
            if (!/^[a-z0-9\-]+$/.test(v)) { setInvalid(nameField, 'Lowercase letters, numbers, hyphens only'); return false; }
            if (RESERVED.includes(v)) { setInvalid(nameField, '"' + v + '" is reserved'); return false; }
            clearInvalid(nameField);
            return true;
        }

        function validateURL() {
            if (!urlField) return true;
            const v = urlField.value.trim();
            if (v === '') { setInvalid(urlField, 'URL is required'); return false; }
            // Replace placeholders so URL can be parsed
            const normalized = v.replace(/\{args\}/g, 'x').replace(/\{\d+\}/g, 'x');
            try {
                const parsed = new URL(normalized);
                if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
                    setInvalid(urlField, 'URL must use http:// or https:// scheme');
                    return false;
                }
            } catch (e) {
                setInvalid(urlField, 'Must be a valid absolute URL (https://...)');
                return false;
            }
            clearInvalid(urlField);
            return true;
        }

        function validateCIDR() {
            if (!cidrField) return true;
            const v = cidrField.value.trim();
            if (v === '') { clearInvalid(cidrField); return true; }
            const lines = v.split('\n').map(s => s.trim()).filter(Boolean);
            const invalid = lines.filter(l => !isValidCIDR(l));
            if (invalid.length > 0) {
                setInvalid(cidrField, 'Invalid CIDR: ' + invalid[0]);
                return false;
            }
            clearInvalid(cidrField);
            return true;
        }

        function setInvalid(el, msg) {
            el.classList.add('field-invalid');
            let tip = el.closest('.form-group').querySelector('.field-error');
            if (!tip) {
                tip = document.createElement('div');
                tip.className = 'field-error';
                const parent = el.closest('.input-prefix') || el;
                parent.parentNode.insertBefore(tip, parent.nextSibling);
            }
            tip.textContent = msg;
            tip.hidden = false;
            // Set row to error state
            const row = el.closest('.form-row');
            if (row) {
                row.classList.remove('row-saved', 'row-editing', 'row-saving');
                row.classList.add('row-error');
            }
        }

        function clearInvalid(el) {
            el.classList.remove('field-invalid');
            const tip = el.closest('.form-group').querySelector('.field-error');
            if (tip) tip.hidden = true;
        }

        function validateAll() {
            const n = validateName();
            const u = validateURL();
            const c = validateCIDR();
            return n && u && c;
        }

        // Attach validation + dirty tracking on input
        linkForm.querySelectorAll('input:not([type=hidden]), textarea').forEach(el => {
            el.addEventListener('input', function() {
                const fieldName = el.name || el.id;
                // Run validation for validated fields
                if (el === nameField) validateName();
                else if (el === urlField) validateURL();
                else if (el === cidrField) validateCIDR();

                // Track dirty state
                if (el.value !== savedValues[el.name]) {
                    dirtyFields.add(fieldName);
                    const row = el.closest('.form-row');
                    if (row && !row.classList.contains('row-error')) {
                        setRowState(row.dataset.field, 'editing');
                    }
                } else {
                    dirtyFields.delete(fieldName);
                    const row = el.closest('.form-row');
                    if (row && !row.classList.contains('row-error')) {
                        setRowState(row.dataset.field, isNew ? null : 'saved');
                    }
                }

                scheduleSave();
            });
        });

        // --- Autosave ---
        function scheduleSave() {
            if (debounceTimer) clearTimeout(debounceTimer);
            debounceTimer = setTimeout(doSave, DEBOUNCE_MS);
        }

        function doSave() {
            if (!validateAll()) return;
            // Don't save if nothing changed
            if (dirtyFields.size === 0 && !isNew) return;
            // For new links, require at least name and URL
            if (isNew) {
                const n = (nameField ? nameField.value.trim() : '');
                const u = (urlField ? urlField.value.trim() : '');
                if (!n || !u) return;
            }

            if (saving) { pendingSave = true; return; }

            saving = true;
            showGlobalStatus('saving');
            // Set dirty rows to saving state
            dirtyFields.forEach(f => setRowState(f, 'saving'));

            const formData = new FormData(linkForm);

            fetch('/admin/save', { method: 'POST', body: formData })
                .then(r => r.json().then(data => ({ ok: r.ok, data })))
                .then(({ ok, data }) => {
                    saving = false;
                    if (ok && data.status === 'ok') {
                        // Update saved values snapshot
                        linkForm.querySelectorAll('input:not([type=hidden]), textarea').forEach(el => {
                            if (el.name) savedValues[el.name] = el.value;
                        });
                        dirtyFields.clear();
                        setAllRowState('saved');
                        showGlobalStatus('saved');

                        // Switch to edit mode if was new
                        const isNewField = linkForm.querySelector('[name="is_new"]');
                        if (isNewField && isNewField.value === 'true') {
                            isNewField.value = 'false';
                            isNew = false;
                            if (nameField) nameField.readOnly = true;
                            const newName = (nameField ? nameField.value.trim() : '');
                            if (newName) {
                                history.replaceState(null, '', '/admin/edit/' + encodeURIComponent(newName));
                            }
                        }
                    } else {
                        showGlobalStatus('error');
                        statusErrorMsg.textContent = data.error || 'Save failed';
                        // Set dirty rows to error
                        dirtyFields.forEach(f => setRowState(f, 'error'));
                    }

                    if (pendingSave) {
                        pendingSave = false;
                        scheduleSave();
                    }
                })
                .catch(() => {
                    saving = false;
                    showGlobalStatus('error');
                    statusErrorMsg.textContent = 'Network error';
                    dirtyFields.forEach(f => setRowState(f, 'error'));
                    if (pendingSave) {
                        pendingSave = false;
                        scheduleSave();
                    }
                });
        }

        // Prevent default form submit, trigger immediate save
        linkForm.addEventListener('submit', function(e) {
            e.preventDefault();
            if (debounceTimer) clearTimeout(debounceTimer);
            doSave();
        });
    }

    // ===== CIDR field: auto-populate with client IP, visibility warning =====
    const cidrFieldForWarning = document.getElementById('cidr_allow');
    if (cidrFieldForWarning) {
        const clientIP = cidrFieldForWarning.dataset.clientIp;
        let cidrWarning = document.createElement('div');
        cidrWarning.className = 'cidr-warning';
        cidrWarning.hidden = true;
        cidrFieldForWarning.closest('.form-group').appendChild(cidrWarning);

        // Auto-fill on first focus if empty
        let firstFocus = true;
        cidrFieldForWarning.addEventListener('focus', function() {
            if (firstFocus && this.value.trim() === '' && clientIP) {
                this.value = clientIP + '/32';
                const len = this.value.length;
                this.setSelectionRange(len, len);
                this.dispatchEvent(new Event('input', { bubbles: true }));
            }
            firstFocus = false;
        });

        function checkCIDRWarning() {
            const val = cidrFieldForWarning.value.trim();
            if (val === '') { cidrWarning.hidden = true; return; }
            if (!clientIP) { cidrWarning.hidden = true; return; }

            const cidrs = val.split('\n').map(s => s.trim()).filter(Boolean);
            const validCidrs = cidrs.filter(isValidCIDR);
            if (validCidrs.length === 0) { cidrWarning.hidden = true; return; }

            const ipInList = validCidrs.some(cidr => ipMatchesCIDR(clientIP, cidr));

            if (!ipInList) {
                cidrWarning.innerHTML = '<span class="warning-icon">&#9888;</span> Your IP <strong>' +
                    esc(clientIP) + '</strong> is not in this list. This link will not be visible to you in admin or suggestions. ' +
                    '<a href="#" class="add-ip-link">Add my IP</a>';
                cidrWarning.hidden = false;
                cidrWarning.querySelector('.add-ip-link').addEventListener('click', function(e) {
                    e.preventDefault();
                    const current = cidrFieldForWarning.value.trim();
                    cidrFieldForWarning.value = current + (current ? '\n' : '') + clientIP + '/32';
                    checkCIDRWarning();
                    cidrFieldForWarning.focus();
                    const len = cidrFieldForWarning.value.length;
                    cidrFieldForWarning.setSelectionRange(len, len);
                    cidrFieldForWarning.dispatchEvent(new Event('input', { bubbles: true }));
                });
            } else {
                cidrWarning.hidden = true;
            }
        }

        cidrFieldForWarning.addEventListener('input', checkCIDRWarning);
        checkCIDRWarning();
    }

    function esc(s) {
        if (!s) return '';
        const d = document.createElement('div');
        d.textContent = s;
        return d.innerHTML;
    }
})();
