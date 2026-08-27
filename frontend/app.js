/* ============================================================
 * LinkShort 前端应用逻辑
 * 对接 rest-api-svc 的全部 REST 端点：
 *   POST   /api/v1/shorten
 *   GET    /api/v1/urls/:code?user_id=
 *   DELETE /api/v1/urls/:code?user_id=
 *   GET    /api/v1/users/:userID/urls
 *   GET    /api/v1/analytics/dashboard
 *   GET    /api/v1/analytics/top-urls
 *   GET    /api/v1/analytics/urls/:code
 * 当后端不可达时自动降级为「演示模式」，保证界面完整可浏览。
 * ============================================================ */

const API_BASE = window.location.origin; // 前端与后端同源部署
const LS_USER_KEY = 'linkshort.userId';

// 运行状态
const state = {
    demoMode: false,      // 后端不可达时开启
    links: [],            // 当前加载的用户链接
    ownerId: '',          // 当前查询的用户 ID
    clicksChart: null,
    deviceChart: null,
    countryChart: null,
    dashboardLoaded: false,
};

/* ------------------------- 工具函数 ------------------------- */

const TOAST_ICONS = {
    success: '<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/>',
    error: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>',
    info: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>',
};

function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.innerHTML = `
        <span class="toast-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">${TOAST_ICONS[type] || TOAST_ICONS.info}</svg>
        </span>
        <span class="toast-msg"></span>`;
    toast.querySelector('.toast-msg').textContent = message;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('leaving');
        setTimeout(() => toast.remove(), 300);
    }, 3200);
}

// created_at / expires_at 均为 Unix 秒级时间戳
function formatDate(ts) {
    if (!ts) return '-';
    const n = Number(ts);
    if (!Number.isFinite(n)) return '-';
    // 兼容秒/毫秒：小于 1e12 视为秒
    const ms = n < 1e12 ? n * 1000 : n;
    const d = new Date(ms);
    if (isNaN(d.getTime())) return '-';
    return d.toLocaleString('zh-CN', { hour12: false });
}

function formatNumber(n) {
    n = Number(n) || 0;
    if (n >= 1e9) return (n / 1e9).toFixed(1).replace(/\.0$/, '') + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, '') + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'k';
    return String(n);
}

function escapeHtml(str) {
    return String(str == null ? '' : str).replace(/[&<>"']/g, c => (
        { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
    ));
}

function buildShortUrl(code) {
    return `${window.location.protocol}//${window.location.host}/${code}`;
}

function toggleLoading(show) {
    document.getElementById('loading').classList.toggle('hidden', !show);
}

async function copyText(text) {
    try {
        if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(text);
        } else {
            const ta = document.createElement('textarea');
            ta.value = text;
            ta.style.position = 'fixed';
            ta.style.opacity = '0';
            document.body.appendChild(ta);
            ta.select();
            document.execCommand('copy');
            ta.remove();
        }
        showToast('已复制到剪贴板！', 'success');
    } catch (e) {
        showToast('复制失败，请手动复制', 'error');
    }
}

/* 统一 fetch 封装：解析 JSON，非 2xx 抛错 */
async function api(path, options = {}) {
    const resp = await fetch(`${API_BASE}${path}`, {
        headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
        ...options,
    });
    let data = null;
    try { data = await resp.json(); } catch (_) { /* 可能无 body */ }
    if (!resp.ok) {
        const msg = (data && (data.error || data.message)) || `请求失败 (${resp.status})`;
        const err = new Error(msg);
        err.status = resp.status;
        throw err;
    }
    return data;
}

/* ------------------------- 视图切换 ------------------------- */
document.querySelectorAll('.nav-link').forEach(btn => {
    btn.addEventListener('click', () => {
        const viewId = btn.getAttribute('data-view');
        document.querySelectorAll('.nav-link').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
        document.getElementById(`${viewId}-view`).classList.add('active');
        if (viewId === 'admin' && !state.dashboardLoaded) {
            loadAdminDashboard();
        }
    });
});

/* ------------------------- 创建短链接 ------------------------- */
const createForm = document.getElementById('createUrlForm');
createForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    const longUrl = document.getElementById('longUrl').value.trim();
    const userId = document.getElementById('userId').value.trim();
    const customAlias = document.getElementById('customAlias').value.trim();

    if (!longUrl || !userId) {
        showToast('请填写原始链接与用户 ID', 'error');
        return;
    }

    const submitBtn = document.getElementById('submitBtn');
    const originalHtml = submitBtn.innerHTML;
    submitBtn.disabled = true;
    submitBtn.innerHTML = '<span class="btn-spinner"></span> 生成中...';

    try {
        const data = await api('/api/v1/shorten', {
            method: 'POST',
            body: JSON.stringify({ long_url: longUrl, user_id: userId, custom_alias: customAlias || undefined }),
        });
        state.demoMode = false;
        localStorage.setItem(LS_USER_KEY, userId);
        showUrlResult(data);
        showToast('短链接创建成功！', 'success');

        // 若正查看同一用户的链接列表，自动刷新
        const ownerInput = document.getElementById('ownerId');
        if (!ownerInput.value.trim()) ownerInput.value = userId;
        if (ownerInput.value.trim() === userId) loadUserLinks(userId, { silent: true });
    } catch (err) {
        // 后端不可达 → 演示模式
        if (err.status === undefined) {
            state.demoMode = true;
            const demo = demoCreate(longUrl, userId, customAlias);
            showUrlResult(demo);
            showToast('后端未连接，已生成演示短链', 'info');
        } else {
            showToast(err.message || '创建失败，请稍后重试', 'error');
        }
    } finally {
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalHtml;
    }
});

function showUrlResult(data) {
    const resultDiv = document.getElementById('urlResult');
    const displayInput = document.getElementById('shortUrlDisplay');
    const shortUrl = data.short_url && /^https?:\/\//.test(data.short_url)
        ? data.short_url
        : buildShortUrl(data.short_code);

    displayInput.value = shortUrl;
    document.getElementById('shortCodeDisplay').textContent = data.short_code;
    document.getElementById('createdAtDisplay').textContent = formatDate(data.created_at);

    // 生成二维码
    const qrBox = document.getElementById('qrCode');
    qrBox.innerHTML = '';
    if (window.QRCode) {
        new QRCode(qrBox, { text: shortUrl, width: 96, height: 96, colorDark: '#0b1120', colorLight: '#ffffff' });
    }

    resultDiv.classList.remove('hidden');
    resultDiv.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

document.getElementById('copyBtn').addEventListener('click', () => {
    copyText(document.getElementById('shortUrlDisplay').value);
});

/* ------------------------- 我的链接列表 ------------------------- */
const ownerInput = document.getElementById('ownerId');
document.getElementById('loadLinksBtn').addEventListener('click', () => {
    const uid = ownerInput.value.trim();
    if (!uid) { showToast('请输入用户 ID', 'error'); return; }
    loadUserLinks(uid);
});
ownerInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); document.getElementById('loadLinksBtn').click(); }
});

document.getElementById('searchLinks').addEventListener('input', (e) => {
    renderLinks(filterLinks(e.target.value.trim().toLowerCase()));
});

function filterLinks(kw) {
    if (!kw) return state.links;
    return state.links.filter(l =>
        (l.short_code || '').toLowerCase().includes(kw) ||
        (l.long_url || '').toLowerCase().includes(kw)
    );
}

async function loadUserLinks(userId, { silent = false } = {}) {
    state.ownerId = userId;
    localStorage.setItem(LS_USER_KEY, userId);
    renderLinksSkeleton();

    try {
        const data = await api(`/api/v1/users/${encodeURIComponent(userId)}/urls?page=1&page_size=50&sort_by=created_at&sort_order=desc`);
        state.demoMode = false;
        state.links = data.urls || [];
        renderLinks(state.links);
        updateLinksCount(state.links.length, data.total_count);
        if (!silent) showToast(`已加载 ${state.links.length} 条链接`, 'success');
    } catch (err) {
        if (err.status === undefined) {
            state.demoMode = true;
            state.links = demoUserLinks(userId);
            renderLinks(state.links);
            updateLinksCount(state.links.length);
            if (!silent) showToast('后端未连接，展示演示数据', 'info');
        } else {
            state.links = [];
            renderLinks([]);
            updateLinksCount(0);
            if (!silent) showToast(err.message || '加载失败', 'error');
        }
    }
}

function updateLinksCount(shown, total) {
    const el = document.getElementById('linksCount');
    if (!shown) { el.textContent = ''; return; }
    el.textContent = total && total !== shown ? `· 共 ${total} 条` : `· ${shown} 条`;
}

function renderLinksSkeleton() {
    const list = document.getElementById('linksList');
    list.innerHTML = Array.from({ length: 6 }).map(() => '<div class="skeleton-card"></div>').join('');
}

function renderLinks(links) {
    const list = document.getElementById('linksList');
    if (!links || links.length === 0) {
        list.innerHTML = `
            <div class="empty-state">
                <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
                    <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
                </svg>
                <p>${state.ownerId ? '该用户暂无短链接' : '输入用户 ID 并点击「加载」查看该用户的短链接'}</p>
            </div>`;
        return;
    }

    list.innerHTML = links.map((l, i) => {
        const shortUrl = l.short_url && /^https?:\/\//.test(l.short_url) ? l.short_url : buildShortUrl(l.short_code);
        const active = l.is_active !== false;
        return `
        <div class="link-card glass-card" style="animation-delay:${i * 40}ms" data-code="${escapeHtml(l.short_code)}">
            <div class="link-card-top">
                <div class="link-short">/${escapeHtml(l.short_code)}</div>
                <span class="link-badge ${active ? 'active' : 'inactive'}">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        ${active ? '<polyline points="20 6 9 17 4 12"/>' : '<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>'}
                    </svg>
                    ${active ? '生效中' : '已停用'}
                </span>
            </div>
            <div class="link-long" title="${escapeHtml(l.long_url)}">${escapeHtml(l.long_url)}</div>
            <div class="link-meta">
                <span class="link-clicks">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
                    ${formatNumber(l.click_count)} 次点击
                </span>
                <span>${formatDate(l.created_at)}</span>
            </div>
            <div class="link-actions">
                <button class="icon-btn" data-act="copy" title="复制短链接">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                </button>
                <button class="icon-btn" data-act="open" title="打开短链接">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                </button>
                <button class="icon-btn" data-act="stats" title="查看分析">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 20V10"/><path d="M12 20V4"/><path d="M6 20v-6"/></svg>
                </button>
                <button class="icon-btn danger" data-act="delete" title="删除">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                </button>
            </div>
        </div>`;
    }).join('');

    // 事件委托
    list.querySelectorAll('.link-card').forEach(card => {
        const code = card.getAttribute('data-code');
        const link = links.find(l => l.short_code === code);
        card.querySelectorAll('[data-act]').forEach(b => {
            b.addEventListener('click', (e) => {
                e.stopPropagation();
                const act = b.getAttribute('data-act');
                if (act === 'copy') copyText(buildShortUrl(code));
                else if (act === 'open') window.open(buildShortUrl(code), '_blank');
                else if (act === 'stats') openLinkModal(link);
                else if (act === 'delete') deleteLink(link);
            });
        });
        card.addEventListener('click', () => openLinkModal(link));
    });
}

async function deleteLink(link) {
    if (!confirm(`确定删除短链接 /${link.short_code} 吗？此操作不可撤销。`)) return;
    try {
        if (state.demoMode) {
            state.links = state.links.filter(l => l.short_code !== link.short_code);
            renderLinks(filterLinks(document.getElementById('searchLinks').value.trim().toLowerCase()));
            updateLinksCount(state.links.length);
            showToast('已删除（演示模式）', 'success');
            return;
        }
        await api(`/api/v1/urls/${encodeURIComponent(link.short_code)}?user_id=${encodeURIComponent(link.user_id || state.ownerId)}`, { method: 'DELETE' });
        state.links = state.links.filter(l => l.short_code !== link.short_code);
        renderLinks(filterLinks(document.getElementById('searchLinks').value.trim().toLowerCase()));
        updateLinksCount(state.links.length);
        showToast('短链接已删除', 'success');
    } catch (err) {
        showToast(err.message || '删除失败', 'error');
    }
}

/* ------------------------- 链接详情模态框 ------------------------- */
const modal = document.getElementById('linkModal');
document.getElementById('closeModal').addEventListener('click', closeModal);
modal.querySelector('.modal-overlay').addEventListener('click', closeModal);
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeModal(); });

function closeModal() { modal.classList.add('hidden'); }

async function openLinkModal(link) {
    if (!link) return;
    const body = document.getElementById('modalBody');
    modal.classList.remove('hidden');
    body.innerHTML = '<div style="text-align:center;padding:2rem"><div class="loading-spinner" style="margin:0 auto"></div></div>';

    const shortUrl = buildShortUrl(link.short_code);
    let stats = null;
    try {
        if (!state.demoMode) {
            stats = await api(`/api/v1/analytics/urls/${encodeURIComponent(link.short_code)}`);
        }
    } catch (_) { /* 分析可能未就绪，忽略 */ }
    if (!stats) stats = demoUrlStats(link);

    const active = link.is_active !== false;
    body.innerHTML = `
        <div class="modal-stats-row">
            <div class="stat-box"><div class="num">${formatNumber(stats.total_clicks ?? link.click_count ?? 0)}</div><div class="cap">总点击量</div></div>
            <div class="stat-box"><div class="num">${formatNumber(stats.unique_clicks ?? 0)}</div><div class="cap">独立访客</div></div>
        </div>
        <div class="modal-detail-grid">
            <div class="detail-item full">
                <div class="label">短链接</div>
                <div class="value link">${escapeHtml(shortUrl)}</div>
            </div>
            <div class="detail-item full">
                <div class="label">原始链接</div>
                <div class="value">${escapeHtml(link.long_url)}</div>
            </div>
            <div class="detail-item"><div class="label">短码</div><div class="value">${escapeHtml(link.short_code)}</div></div>
            <div class="detail-item"><div class="label">状态</div><div class="value" style="color:${active ? '#4ade80' : '#94a3b8'}">${active ? '生效中' : '已停用'}</div></div>
            <div class="detail-item"><div class="label">用户 ID</div><div class="value">${escapeHtml(link.user_id || state.ownerId || '-')}</div></div>
            <div class="detail-item"><div class="label">创建时间</div><div class="value">${formatDate(link.created_at)}</div></div>
            ${link.expires_at ? `<div class="detail-item full"><div class="label">过期时间</div><div class="value">${formatDate(link.expires_at)}</div></div>` : ''}
        </div>
        <div class="modal-actions">
            <button class="btn btn-ghost" id="modalCopy">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                复制链接
            </button>
            <button class="btn btn-ghost" id="modalOpen">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                访问
            </button>
            <button class="btn btn-danger" id="modalDelete">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/></svg>
                删除
            </button>
        </div>`;

    body.querySelector('#modalCopy').addEventListener('click', () => copyText(shortUrl));
    body.querySelector('#modalOpen').addEventListener('click', () => window.open(shortUrl, '_blank'));
    body.querySelector('#modalDelete').addEventListener('click', () => { closeModal(); deleteLink(link); });
}

/* ------------------------- 管理仪表板 ------------------------- */
async function loadAdminDashboard() {
    try {
        const [dashboard, top] = await Promise.all([
            api('/api/v1/analytics/dashboard'),
            api('/api/v1/analytics/top-urls?limit=10&sort_by=clicks'),
        ]);
        state.demoMode = false;
        renderDashboard(dashboard, top.urls || []);
        state.dashboardLoaded = true;
    } catch (err) {
        // 降级演示
        state.demoMode = true;
        const demo = demoDashboard();
        renderDashboard(demo.dashboard, demo.top);
        state.dashboardLoaded = true;
        if (err.status !== undefined) {
            showToast(err.message || '仪表板加载失败，展示演示数据', 'info');
        } else {
            showToast('后端未连接，展示演示数据', 'info');
        }
    }
}

function renderDashboard(d, topUrls) {
    animateValue('totalUrls', d.total_urls || 0);
    animateValue('totalClicks', d.total_clicks || 0);
    animateValue('uniqueClicks', d.unique_clicks || 0);
    document.getElementById('activeUrls').textContent = formatNumber(d.active_urls || 0);
    const ctr = d.total_urls ? (d.total_clicks / d.total_urls) : 0;
    document.getElementById('ctr').textContent = ctr.toFixed(1);

    renderClicksChart(d.click_timeline || []);
    renderDeviceChart(d.device_breakdown || []);
    renderCountryChart(d.top_countries || []);
    renderTopLinks(topUrls);
}

function animateValue(id, target) {
    const el = document.getElementById(id);
    target = Number(target) || 0;
    const dur = 900, start = performance.now();
    function step(now) {
        const p = Math.min((now - start) / dur, 1);
        const eased = 1 - Math.pow(1 - p, 3);
        el.textContent = formatNumber(Math.round(target * eased));
        if (p < 1) requestAnimationFrame(step);
        else el.textContent = formatNumber(target);
    }
    requestAnimationFrame(step);
}

const CHART_FONT = { family: 'Inter', size: 12 };

function renderClicksChart(timeline) {
    const ctx = document.getElementById('clicksChart').getContext('2d');
    if (state.clicksChart) state.clicksChart.destroy();

    const labels = timeline.map(p => {
        const d = new Date((p.timestamp < 1e12 ? p.timestamp * 1000 : p.timestamp));
        return d.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric' });
    });
    const clicks = timeline.map(p => p.clicks);
    const uniques = timeline.map(p => p.unique_clicks);

    const grad = ctx.createLinearGradient(0, 0, 0, 280);
    grad.addColorStop(0, 'rgba(0, 212, 255, 0.35)');
    grad.addColorStop(1, 'rgba(0, 212, 255, 0)');

    state.clicksChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels,
            datasets: [
                { label: '总点击', data: clicks, borderColor: '#00d4ff', backgroundColor: grad, tension: 0.4, fill: true, borderWidth: 3, pointRadius: 0, pointHoverRadius: 5, pointBackgroundColor: '#fff' },
                { label: '独立访客', data: uniques, borderColor: '#c084fc', backgroundColor: 'transparent', tension: 0.4, fill: false, borderWidth: 2, borderDash: [5, 4], pointRadius: 0, pointHoverRadius: 5 },
            ],
        },
        options: {
            responsive: true, maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: {
                legend: { display: true, labels: { color: '#94a3b8', font: CHART_FONT, usePointStyle: true, boxWidth: 8 } },
                tooltip: { backgroundColor: 'rgba(11,17,32,0.95)', borderColor: 'rgba(148,163,184,0.2)', borderWidth: 1, titleColor: '#fff', bodyColor: '#cbd5e1', padding: 10 },
            },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#64748b', font: CHART_FONT } },
                x: { grid: { display: false }, ticks: { color: '#64748b', font: CHART_FONT, maxRotation: 0, autoSkip: true, maxTicksLimit: 8 } },
            },
        },
    });
}

function renderDeviceChart(devices) {
    const ctx = document.getElementById('deviceChart').getContext('2d');
    if (state.deviceChart) state.deviceChart.destroy();
    const labels = devices.map(d => d.device_type);
    const data = devices.map(d => d.clicks);
    const colors = ['#00d4ff', '#9d4edd', '#22c55e', '#f59e0b', '#f5576c'];

    state.deviceChart = new Chart(ctx, {
        type: 'doughnut',
        data: { labels, datasets: [{ data, backgroundColor: colors, borderColor: 'rgba(11,17,32,0.8)', borderWidth: 3, hoverOffset: 6 }] },
        options: {
            responsive: true, maintainAspectRatio: false, cutout: '62%',
            plugins: {
                legend: { position: 'right', labels: { color: '#94a3b8', font: CHART_FONT, usePointStyle: true, boxWidth: 8, padding: 12 } },
                tooltip: { backgroundColor: 'rgba(11,17,32,0.95)', padding: 10, callbacks: { label: (c) => ` ${c.label}: ${formatNumber(c.parsed)}` } },
            },
        },
    });
}

function renderCountryChart(countries) {
    const ctx = document.getElementById('countryChart').getContext('2d');
    if (state.countryChart) state.countryChart.destroy();
    const top = countries.slice(0, 5);
    const labels = top.map(c => c.country);
    const data = top.map(c => c.clicks);

    const grad = ctx.createLinearGradient(0, 0, 400, 0);
    grad.addColorStop(0, '#9d4edd');
    grad.addColorStop(1, '#00d4ff');

    state.countryChart = new Chart(ctx, {
        type: 'bar',
        data: { labels, datasets: [{ label: '点击量', data, backgroundColor: grad, borderRadius: 8, barThickness: 22 }] },
        options: {
            indexAxis: 'y', responsive: true, maintainAspectRatio: false,
            plugins: { legend: { display: false }, tooltip: { backgroundColor: 'rgba(11,17,32,0.95)', padding: 10 } },
            scales: {
                x: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#64748b', font: CHART_FONT } },
                y: { grid: { display: false }, ticks: { color: '#cbd5e1', font: CHART_FONT } },
            },
        },
    });
}

function renderTopLinks(urls) {
    const list = document.getElementById('topLinksList');
    if (!urls || urls.length === 0) {
        list.innerHTML = '<div class="empty-state" style="padding:2rem"><p>暂无热门链接数据</p></div>';
        return;
    }
    list.innerHTML = urls.slice(0, 10).map((u, i) => `
        <div class="top-link-item" data-code="${escapeHtml(u.short_code)}">
            <div class="rank-badge">${i + 1}</div>
            <div class="top-link-info">
                <div class="top-link-code">/${escapeHtml(u.short_code)}</div>
                <div class="top-link-sub">独立访客 ${formatNumber(u.unique_clicks || 0)}</div>
            </div>
            <div class="top-link-clicks">${formatNumber(u.total_clicks || 0)}<span>点击</span></div>
        </div>`).join('');

    list.querySelectorAll('.top-link-item').forEach(item => {
        item.addEventListener('click', () => {
            const code = item.getAttribute('data-code');
            const u = urls.find(x => x.short_code === code);
            openLinkModal({ short_code: code, long_url: u.long_url || '(未知)', click_count: u.total_clicks, created_at: u.created_at, is_active: true });
        });
    });
}

document.getElementById('chartTimeRange').addEventListener('change', async (e) => {
    const range = e.target.value;
    const now = Math.floor(Date.now() / 1000);
    const spans = { '24h': 86400, '7d': 7 * 86400, '30d': 30 * 86400 };
    const start = now - (spans[range] || spans['7d']);
    try {
        if (state.demoMode) throw new Error('demo');
        const d = await api(`/api/v1/analytics/dashboard?start_time=${new Date(start * 1000).toISOString()}&end_time=${new Date(now * 1000).toISOString()}`);
        renderClicksChart(d.click_timeline || []);
    } catch (_) {
        renderClicksChart(demoTimeline(range));
    }
});

/* ------------------------- 演示模式数据 ------------------------- */
function randCode(n = 6) {
    const s = 'abcdefghijkmnpqrstuvwxyz23456789';
    return Array.from({ length: n }, () => s[Math.floor(Math.random() * s.length)]).join('');
}

function demoCreate(longUrl, userId, alias) {
    return { short_code: alias || randCode(), short_url: buildShortUrl(alias || randCode()), long_url: longUrl, created_at: Math.floor(Date.now() / 1000), user_id: userId };
}

function demoUserLinks(userId) {
    const samples = [
        'https://github.com/go-systems-lab/go-url-shortener',
        'https://go-micro.dev/v5/getting-started',
        'https://clickhouse.com/docs/en/intro',
        'https://redis.io/docs/latest/develop/data-types/',
        'https://www.postgresql.org/docs/current/indexes.html',
        'https://prometheus.io/docs/introduction/overview/',
        'https://opentelemetry.io/docs/languages/go/',
        'https://nats.io/about/',
    ];
    const now = Math.floor(Date.now() / 1000);
    return samples.map((u, i) => ({
        short_code: randCode(), short_url: '', long_url: u, user_id: userId,
        created_at: now - i * 3600 * 9, click_count: Math.floor(Math.random() * 4000) + 30,
        is_active: Math.random() > 0.15,
    }));
}

function demoUrlStats(link) {
    const total = link.click_count || Math.floor(Math.random() * 2000) + 100;
    return { short_code: link.short_code, total_clicks: total, unique_clicks: Math.floor(total * 0.68) };
}

function demoTimeline(range = '7d') {
    const points = range === '24h' ? 24 : range === '30d' ? 30 : 7;
    const now = Date.now();
    const stepMs = range === '24h' ? 3600e3 : 86400e3;
    let base = 400;
    return Array.from({ length: points }, (_, i) => {
        base = Math.max(60, base + (Math.random() - 0.45) * 260);
        const clicks = Math.round(base + Math.random() * 200);
        return { timestamp: Math.floor((now - (points - 1 - i) * stepMs) / 1000), clicks, unique_clicks: Math.round(clicks * 0.66) };
    });
}

function demoDashboard() {
    const timeline = demoTimeline('7d');
    const total = timeline.reduce((s, p) => s + p.clicks, 0) * 6;
    const dashboard = {
        total_urls: 1248, total_clicks: total, unique_clicks: Math.round(total * 0.64), active_urls: 1103,
        click_timeline: timeline,
        device_breakdown: [
            { device_type: 'Desktop', clicks: Math.round(total * 0.52) },
            { device_type: 'Mobile', clicks: Math.round(total * 0.37) },
            { device_type: 'Tablet', clicks: Math.round(total * 0.08) },
            { device_type: 'Other', clicks: Math.round(total * 0.03) },
        ],
        top_countries: [
            { country: '中国', clicks: Math.round(total * 0.41) },
            { country: '美国', clicks: Math.round(total * 0.22) },
            { country: '日本', clicks: Math.round(total * 0.13) },
            { country: '德国', clicks: Math.round(total * 0.08) },
            { country: '新加坡', clicks: Math.round(total * 0.06) },
        ],
    };
    const top = Array.from({ length: 10 }, () => {
        const t = Math.floor(Math.random() * 8000) + 200;
        return { short_code: randCode(), total_clicks: t, unique_clicks: Math.round(t * 0.6), long_url: 'https://example.com/' + randCode(10) };
    }).sort((a, b) => b.total_clicks - a.total_clicks);
    return { dashboard, top };
}

/* ------------------------- 初始化 ------------------------- */
(function init() {
    const saved = localStorage.getItem(LS_USER_KEY);
    if (saved) {
        document.getElementById('userId').value = saved;
        document.getElementById('ownerId').value = saved;
    }
})();
