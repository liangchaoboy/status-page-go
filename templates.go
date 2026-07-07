package main

const loginHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>登录 - 服务状态监控</title>
  <style>
    :root {
      --bg-primary: #0f172a;
      --bg-secondary: #1e293b;
      --bg-card: #334155;
      --text-primary: #f1f5f9;
      --text-secondary: #94a3b8;
      --border-color: #475569;
      --accent: #3b82f6;
      --danger: #ef4444;
    }
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1rem;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: var(--bg-primary);
      color: var(--text-primary);
    }
    .login-panel {
      width: 100%;
      max-width: 380px;
      background: var(--bg-secondary);
      border: 1px solid var(--border-color);
      border-radius: 8px;
      padding: 1.5rem;
    }
    h1 { font-size: 1.25rem; margin-bottom: 0.25rem; }
    .subtitle { color: var(--text-secondary); font-size: 0.875rem; margin-bottom: 1.5rem; }
    label { display: block; font-size: 0.875rem; margin-bottom: 0.5rem; color: var(--text-secondary); }
    input {
      width: 100%;
      padding: 0.75rem 0.875rem;
      border-radius: 6px;
      border: 1px solid var(--border-color);
      background: var(--bg-card);
      color: var(--text-primary);
      font-size: 0.95rem;
      margin-bottom: 1rem;
    }
    input:focus { outline: 2px solid rgba(59, 130, 246, 0.45); border-color: var(--accent); }
    button {
      width: 100%;
      padding: 0.75rem 1rem;
      border: 0;
      border-radius: 6px;
      background: var(--accent);
      color: #fff;
      font-weight: 600;
      cursor: pointer;
    }
    button:hover { background: #2563eb; }
    .error {
      padding: 0.75rem;
      border: 1px solid rgba(239, 68, 68, 0.45);
      border-radius: 6px;
      background: rgba(239, 68, 68, 0.12);
      color: #fecaca;
      font-size: 0.875rem;
      margin-bottom: 1rem;
    }
  </style>
</head>
<body>
  <main class="login-panel">
    <h1>服务状态监控</h1>
    <p class="subtitle">登录后查看状态页和管理接口</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="POST" action="/login">
      <input type="hidden" name="next" value="{{.Next}}">
      <label for="username">用户名</label>
      <input id="username" name="username" type="text" autocomplete="username" required autofocus>
      <label for="password">密码</label>
      <input id="password" name="password" type="password" autocomplete="current-password" required>
      <button type="submit">登录</button>
    </form>
  </main>
</body>
</html>`

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>服务状态监控</title>
  <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
  <style>
    :root {
      --color-operational: #22c55e;
      --color-degraded: #f59e0b;
      --color-partial: #f97316;
      --color-outage: #ef4444;
      --color-unknown: #6b7280;
      --bg-primary: #0f172a;
      --bg-secondary: #1e293b;
      --bg-card: #334155;
      --text-primary: #f1f5f9;
      --text-secondary: #94a3b8;
      --border-color: #475569;
    }

    * { margin: 0; padding: 0; box-sizing: border-box; }

    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: var(--bg-primary);
      color: var(--text-primary);
      min-height: 100vh;
      padding: 2rem;
    }

    .container { max-width: 1200px; margin: 0 auto; }

    header { text-align: center; margin-bottom: 3rem; }
    header h1 { font-size: 2rem; margin-bottom: 0.5rem; }
    header p { color: var(--text-secondary); }

    .status-hero {
      background: var(--bg-secondary);
      border-radius: 16px;
      padding: 2.5rem;
      text-align: center;
      margin-bottom: 2rem;
      border: 1px solid var(--border-color);
    }

    .status-indicator {
      display: inline-flex;
      align-items: center;
      gap: 12px;
      font-size: 1.5rem;
      font-weight: 600;
      margin-bottom: 1rem;
    }

    .status-dot {
      width: 16px;
      height: 16px;
      border-radius: 50%;
      animation: pulse 2s infinite;
    }

    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.5; }
    }

    .status-dot.operational { background: var(--color-operational); }
    .status-dot.degraded { background: var(--color-degraded); }
    .status-dot.partial_outage { background: var(--color-partial); }
    .status-dot.major_outage { background: var(--color-outage); }
    .status-dot.unknown { background: var(--color-unknown); }

    .status-message { color: var(--text-secondary); font-size: 1rem; }
    .last-updated { color: var(--text-secondary); font-size: 0.875rem; margin-top: 1rem; }

    .metrics-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
      gap: 1.5rem;
      margin-bottom: 2rem;
    }

    .metric-card {
      background: var(--bg-secondary);
      border-radius: 12px;
      padding: 1.5rem;
      border: 1px solid var(--border-color);
    }

    .metric-card h3 {
      font-size: 0.875rem;
      color: var(--text-secondary);
      margin-bottom: 0.5rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }

    .metric-value { font-size: 2.5rem; font-weight: 700; margin-bottom: 0.25rem; }
    .metric-value.good { color: var(--color-operational); }
    .metric-value.warning { color: var(--color-degraded); }
    .metric-value.bad { color: var(--color-outage); }
    .metric-sub { font-size: 0.875rem; color: var(--text-secondary); }

    .chart-section {
      background: var(--bg-secondary);
      border-radius: 12px;
      padding: 1.5rem;
      margin-bottom: 2rem;
      border: 1px solid var(--border-color);
    }

    .chart-section h2 { font-size: 1.125rem; margin-bottom: 1rem; display: flex; align-items: center; }
    .chart-container { height: 300px; position: relative; }

    .time-btn {
      padding: 0.375rem 0.75rem;
      border-radius: 6px;
      border: 1px solid var(--border-color);
      background: transparent;
      color: var(--text-secondary);
      font-size: 0.75rem;
      cursor: pointer;
      transition: all 0.2s;
    }
    .time-btn:hover { background: var(--bg-card); color: var(--text-primary); }
    .time-btn.active { background: #3b82f6; color: white; border-color: #3b82f6; }

    .config-section, .webhook-section, .alerts-section {
      background: var(--bg-secondary);
      border-radius: 12px;
      padding: 1.5rem;
      border: 1px solid var(--border-color);
      margin-bottom: 2rem;
    }

    .config-section h2, .webhook-section h2, .alerts-section h2 {
      font-size: 1.125rem;
      margin-bottom: 1rem;
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .webhook-form {
      display: flex;
      gap: 0.75rem;
      margin-bottom: 1.5rem;
      flex-wrap: wrap;
    }

    .webhook-form input, .webhook-form select {
      flex: 1;
      min-width: 200px;
      padding: 0.75rem 1rem;
      border-radius: 8px;
      border: 1px solid var(--border-color);
      background: var(--bg-card);
      color: var(--text-primary);
      font-size: 0.875rem;
    }

    .webhook-form input::placeholder { color: var(--text-secondary); }
    .webhook-form select { max-width: 180px; cursor: pointer; }

    .config-form {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 0.75rem;
      align-items: end;
    }
    .config-field label {
      display: block;
      font-size: 0.75rem;
      color: var(--text-secondary);
      margin-bottom: 0.35rem;
    }
    .config-field input {
      width: 100%;
      padding: 0.75rem 1rem;
      border-radius: 8px;
      border: 1px solid var(--border-color);
      background: var(--bg-card);
      color: var(--text-primary);
      font-size: 0.875rem;
    }
    .config-form button {
      padding: 0.75rem 1.5rem;
      border-radius: 8px;
      border: none;
      background: #3b82f6;
      color: white;
      font-weight: 500;
      cursor: pointer;
      min-height: 43px;
    }

    .webhook-form button {
      padding: 0.75rem 1.5rem;
      border-radius: 8px;
      border: none;
      background: #3b82f6;
      color: white;
      font-weight: 500;
      cursor: pointer;
      transition: background 0.2s;
    }

    .webhook-form button:hover { background: #2563eb; }

    .webhook-list { display: flex; flex-direction: column; gap: 0.75rem; }

    .webhook-item {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 1rem;
      background: var(--bg-card);
      border-radius: 8px;
      gap: 1rem;
    }

    .webhook-info { flex: 1; min-width: 0; }
    .webhook-name { font-weight: 500; margin-bottom: 0.25rem; }
    .webhook-url {
      font-size: 0.75rem;
      color: var(--text-secondary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .webhook-meta { font-size: 0.75rem; color: var(--text-secondary); }
    .webhook-actions { display: flex; gap: 0.5rem; }

    .btn-sm {
      padding: 0.5rem 0.75rem;
      border-radius: 6px;
      border: none;
      font-size: 0.75rem;
      cursor: pointer;
      transition: all 0.2s;
    }

    .btn-test {
      background: var(--bg-secondary);
      color: var(--text-primary);
      border: 1px solid var(--border-color);
    }
    .btn-test:hover { background: var(--border-color); }

    .btn-delete {
      background: transparent;
      color: var(--color-outage);
      border: 1px solid var(--color-outage);
    }
    .btn-delete:hover { background: var(--color-outage); color: white; }

    .empty-state { text-align: center; padding: 2rem; color: var(--text-secondary); }

    .alert-item {
      display: flex;
      align-items: flex-start;
      gap: 1rem;
      padding: 1rem;
      background: var(--bg-card);
      border-radius: 8px;
      margin-bottom: 0.75rem;
    }
    .alert-item:last-child { margin-bottom: 0; }

    .alert-type {
      padding: 0.25rem 0.5rem;
      border-radius: 4px;
      font-size: 0.75rem;
      font-weight: 500;
      text-transform: uppercase;
    }
    .alert-type.alert { background: rgba(239, 68, 68, 0.2); color: var(--color-outage); }
    .alert-type.recovery { background: rgba(34, 197, 94, 0.2); color: var(--color-operational); }
    .alert-type.test { background: rgba(59, 130, 246, 0.2); color: #3b82f6; }

    .alert-content { flex: 1; }
    .alert-message { margin-bottom: 0.25rem; }
    .alert-time { font-size: 0.75rem; color: var(--text-secondary); }

    .refresh-btn {
      position: fixed;
      bottom: 2rem;
      right: 2rem;
      width: 56px;
      height: 56px;
      border-radius: 50%;
      background: #3b82f6;
      color: white;
      border: none;
      cursor: pointer;
      box-shadow: 0 4px 12px rgba(59, 130, 246, 0.4);
      display: flex;
      align-items: center;
      justify-content: center;
      transition: all 0.2s;
    }
    .refresh-btn:hover { transform: scale(1.1); }
    .refresh-btn.loading svg { animation: spin 1s linear infinite; }

    @keyframes spin {
      from { transform: rotate(0deg); }
      to { transform: rotate(360deg); }
    }

    @media (max-width: 640px) {
      body { padding: 1rem; }
      .status-hero { padding: 1.5rem; }
      .metric-value { font-size: 2rem; }
    }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>🌐 服务状态监控</h1>
      <p>实时监控系统可用性状态 · <a href="/api-docs" style="color: #3b82f6;">API 文档</a> · <a href="/logout" style="color: #3b82f6;">退出登录</a></p>
    </header>

    <div class="status-hero">
      <div class="status-indicator">
        <span class="status-dot" id="statusDot"></span>
        <span id="statusText">加载中...</span>
      </div>
      <p class="status-message" id="statusMessage">正在获取状态信息</p>
      <p class="last-updated">最后更新: <span id="lastUpdated">-</span></p>
    </div>

    <div class="metrics-grid">
      <div class="metric-card">
        <h3>5 分钟错误率</h3>
        <div class="metric-value" id="error5m">-</div>
        <p class="metric-sub" id="time5m">-</p>
      </div>
      <div class="metric-card">
        <h3>30 分钟错误率</h3>
        <div class="metric-value" id="error30m">-</div>
        <p class="metric-sub" id="time30m">-</p>
      </div>
      <div class="metric-card">
        <h3>1 小时错误率</h3>
        <div class="metric-value" id="error60m">-</div>
        <p class="metric-sub" id="time60m">-</p>
      </div>
      <div class="metric-card">
        <h3>6 小时错误率</h3>
        <div class="metric-value" id="error360m">-</div>
        <p class="metric-sub" id="time360m">-</p>
      </div>
    </div>

    <div class="chart-section">
      <h2>📈 错误率趋势
        <div style="margin-left: auto; display: flex; gap: 0.5rem;">
          <button class="time-btn active" data-range="24" onclick="setTimeRange(24)">2小时</button>
          <button class="time-btn" data-range="72" onclick="setTimeRange(72)">6小时</button>
          <button class="time-btn" data-range="288" onclick="setTimeRange(288)">24小时</button>
        </div>
      </h2>
      <p id="chartHint" style="display:none; font-size:0.8rem; color:var(--color-degraded); margin-bottom:0.75rem;"></p>
      <div class="chart-container">
        <canvas id="errorChart"></canvas>
      </div>
    </div>

    <div class="config-section">
      <h2>⚙️ 告警规则</h2>
      <form class="config-form" id="configForm">
        <div class="config-field">
          <label for="alertThresholdPercent">错误率阈值 (%)</label>
          <input id="alertThresholdPercent" name="alertThresholdPercent" type="number" min="0" max="100" step="0.01" required>
        </div>
        <div class="config-field">
          <label for="alertConsecutivePoints">连续采样点</label>
          <input id="alertConsecutivePoints" name="alertConsecutivePoints" type="number" min="1" step="1" required>
        </div>
        <button type="submit">保存</button>
      </form>
    </div>

    <div class="webhook-section">
      <h2>🔔 告警推送配置</h2>
      <form class="webhook-form" id="webhookForm">
        <input type="text" name="name" placeholder="名称（可选）">
        <select name="type" title="Webhook 类型">
          <option value="auto">自动识别</option>
          <option value="generic">通用 JSON</option>
          <option value="wecom">企业微信机器人</option>
        </select>
        <input type="url" name="url" placeholder="Webhook URL" required>
        <input type="text" name="secret" placeholder="Secret（可选）">
        <button type="submit">添加</button>
      </form>
      <div class="webhook-list" id="webhookList">
        <div class="empty-state">暂无配置的 Webhook</div>
      </div>
    </div>

    <div class="alerts-section">
      <h2>📋 告警历史</h2>
      <div id="alertsList">
        <div class="empty-state">暂无告警记录</div>
      </div>
    </div>
  </div>

  <button class="refresh-btn" id="refreshBtn" title="刷新数据">
    <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/>
      <path d="M21 3v5h-5"/>
    </svg>
  </button>

  <script>
    const STATUS_MAP = {
      operational: { text: '正常运行', color: 'operational' },
      degraded: { text: '性能下降', color: 'degraded' },
      partial_outage: { text: '部分中断', color: 'partial_outage' },
      major_outage: { text: '严重故障', color: 'major_outage' },
      unknown: { text: '状态未知', color: 'unknown' }
    };

    let errorChart = null;
    let currentTimeRange = 24; // 默认 2 小时（5分钟粒度，24条）
    let historyCache = [];
    let alertThreshold = 0.05;
    let alertConsecutivePoints = 2;

    function initChart() {
      const ctx = document.getElementById('errorChart').getContext('2d');
      errorChart = new Chart(ctx, {
        type: 'line',
        data: {
          labels: [],
          datasets: [{
            label: '错误率 %',
            data: [],
            borderColor: '#3b82f6',
            backgroundColor: 'rgba(59, 130, 246, 0.1)',
            fill: true,
            tension: 0.4,
            pointRadius: 3,
            pointHoverRadius: 6
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: { legend: { display: false }, tooltip: { callbacks: {
            title: items => items.length ? items[0].label + ' 前 5 分钟窗口' : '',
            label: ctx => '错误率: ' + ctx.parsed.y.toFixed(2) + '%'
          } } },
          scales: {
            x: { grid: { color: 'rgba(71, 85, 105, 0.3)' }, ticks: { color: '#94a3b8', maxTicksLimit: 10 } },
            y: { min: 0, grid: { color: 'rgba(71, 85, 105, 0.3)' }, ticks: { color: '#94a3b8', callback: v => v + '%' } }
          }
        }
      });
    }

    function updateChart(history) {
      if (!errorChart || !history.length) return;
      historyCache = history;
      renderChart();
    }

    function renderChart() {
      if (!errorChart || !historyCache.length) return;
      // 按 5 分钟粒度切片：2小时=24点, 6小时=72点, 24小时=288点
      const data = historyCache.slice(-currentTimeRange);
      // 数据不足提示：实际点数 < 所选区间时，说明采集时间还不够
      const covered = data.length * 5; // 已覆盖的分钟数
      const wanted = currentTimeRange * 5;
      const hint = document.getElementById('chartHint');
      if (data.length < currentTimeRange) {
        hint.textContent = '数据累积中：已有 ' + data.length + '/' + currentTimeRange +
          ' 个采样点（约 ' + (covered / 60).toFixed(1) + '/' + (wanted / 60) + ' 小时）';
        hint.style.display = 'block';
      } else {
        hint.style.display = 'none';
      }
      // 根据数据量调整时间格式（超过 6 小时显示日期）
      const showDate = currentTimeRange > 72;
      const labels = data.map(h => {
        const d = new Date(h.timestamp);
        if (showDate) {
          return d.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric' }) + ' ' + d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        }
        return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
      });
      const values = data.map(h => (h.error_ratio ?? 0) * 100);
      errorChart.data.labels = labels;
      errorChart.data.datasets[0].data = values;
      // 根据数据量调整点的大小
      errorChart.data.datasets[0].pointRadius = currentTimeRange > 72 ? 2 : 4;
      errorChart.options.scales.x.ticks.maxTicksLimit = currentTimeRange > 72 ? 8 : 12;
      errorChart.update('none');
    }

    function setTimeRange(range) {
      currentTimeRange = range;
      document.querySelectorAll('.time-btn').forEach(btn => {
        btn.classList.toggle('active', parseInt(btn.dataset.range) === range);
      });
      renderChart();
    }

    function getErrorClass(ratio) {
      if (ratio === null || ratio === undefined) return '';
      if (ratio <= alertThreshold) return 'good';
      if (ratio < 0.2) return 'warning';
      return 'bad';
    }

    function formatTimeRange(tr) {
      if (!tr) return '-';
      return new Date(tr.start_time).toLocaleTimeString('zh-CN') + ' - ' + new Date(tr.end_time).toLocaleTimeString('zh-CN');
    }

    async function updateStatus() {
      try {
        const [statusRes, historyRes] = await Promise.all([fetch('/api/status'), fetch('/api/status/history?limit=288')]);
        const statusData = await statusRes.json();
        const historyData = await historyRes.json();

        if (statusData.code === 0 && statusData.data.current) {
          alertThreshold = statusData.data.config?.alertThreshold ?? alertThreshold;
          alertConsecutivePoints = statusData.data.config?.alertConsecutivePoints ?? alertConsecutivePoints;
          syncConfigForm();
          const { current } = statusData.data;
          const overall = current.overall || { status: 'unknown', message: '状态未知' };
          const statusInfo = STATUS_MAP[overall.status] || STATUS_MAP.unknown;

          document.getElementById('statusDot').className = 'status-dot ' + statusInfo.color;
          document.getElementById('statusText').textContent = statusInfo.text;
          document.getElementById('statusMessage').textContent = overall.message;
          document.getElementById('lastUpdated').textContent = new Date(current.timestamp).toLocaleString('zh-CN');

          if (current.status5m) {
            const r = current.status5m.error_ratio ?? 0;
            document.getElementById('error5m').textContent = (r * 100).toFixed(2) + '%';
            document.getElementById('error5m').className = 'metric-value ' + getErrorClass(r);
            document.getElementById('time5m').textContent = formatTimeRange(current.status5m.time_range);
          }
          if (current.status30m) {
            const r = current.status30m.error_ratio ?? 0;
            document.getElementById('error30m').textContent = (r * 100).toFixed(2) + '%';
            document.getElementById('error30m').className = 'metric-value ' + getErrorClass(r);
            document.getElementById('time30m').textContent = formatTimeRange(current.status30m.time_range);
          }
          if (current.status60m) {
            const r = current.status60m.error_ratio ?? 0;
            document.getElementById('error60m').textContent = (r * 100).toFixed(2) + '%';
            document.getElementById('error60m').className = 'metric-value ' + getErrorClass(r);
            document.getElementById('time60m').textContent = formatTimeRange(current.status60m.time_range);
          }
          if (current.status360m) {
            const r = current.status360m.error_ratio ?? 0;
            document.getElementById('error360m').textContent = (r * 100).toFixed(2) + '%';
            document.getElementById('error360m').className = 'metric-value ' + getErrorClass(r);
            document.getElementById('time360m').textContent = formatTimeRange(current.status360m.time_range);
          }
        }

        if (historyData.code === 0) updateChart(historyData.data);
      } catch (e) { console.error('Failed to fetch status:', e); }
    }

    async function loadWebhooks() {
      try {
        const res = await fetch('/api/webhooks');
        const data = await res.json();
        const list = document.getElementById('webhookList');
        if (data.code === 0 && data.data.length > 0) {
          list.innerHTML = data.data.map(w => '<div class="webhook-item"><div class="webhook-info"><div class="webhook-name">' + escapeHtml(w.name) + '</div><div class="webhook-url">' + escapeHtml(w.url) + '</div><div class="webhook-meta">' + webhookTypeLabel(w.type) + ' · 已触发 ' + w.triggerCount + ' 次' + (w.lastTriggered ? ' · 最后: ' + new Date(w.lastTriggered).toLocaleString('zh-CN') : '') + '</div></div><div class="webhook-actions"><button class="btn-sm btn-test" onclick="testWebhook(\'' + w.id + '\')">测试</button><button class="btn-sm btn-delete" onclick="deleteWebhook(\'' + w.id + '\')">删除</button></div></div>').join('');
        } else {
          list.innerHTML = '<div class="empty-state">暂无配置的 Webhook</div>';
        }
      } catch (e) { console.error('Failed to load webhooks:', e); }
    }

    async function loadAlerts() {
      try {
        const res = await fetch('/api/alerts?limit=10');
        const data = await res.json();
        const list = document.getElementById('alertsList');
        if (data.code === 0 && data.data.length > 0) {
          list.innerHTML = data.data.slice().reverse().map(a => '<div class="alert-item"><span class="alert-type ' + a.type + '">' + a.type + '</span><div class="alert-content"><div class="alert-message">' + escapeHtml(a.message) + '</div><div class="alert-time">' + new Date(a.timestamp).toLocaleString('zh-CN') + '</div></div></div>').join('');
        } else {
          list.innerHTML = '<div class="empty-state">暂无告警记录</div>';
        }
      } catch (e) { console.error('Failed to load alerts:', e); }
    }

    function syncConfigForm() {
      document.getElementById('alertThresholdPercent').value = (alertThreshold * 100).toFixed(2);
      document.getElementById('alertConsecutivePoints').value = alertConsecutivePoints;
    }

    async function loadConfig() {
      try {
        const res = await fetch('/api/config');
        const data = await res.json();
        if (data.code === 0) {
          alertThreshold = data.data.alertThreshold ?? alertThreshold;
          alertConsecutivePoints = data.data.alertConsecutivePoints ?? alertConsecutivePoints;
          syncConfigForm();
        }
      } catch (e) { console.error('Failed to load config:', e); }
    }

    document.getElementById('configForm').addEventListener('submit', async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const thresholdPercent = parseFloat(fd.get('alertThresholdPercent'));
      const points = parseInt(fd.get('alertConsecutivePoints'), 10);
      try {
        const res = await fetch('/api/config', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ alertThreshold: thresholdPercent / 100, alertConsecutivePoints: points })
        });
        const data = await res.json();
        if (data.code === 0) {
          alertThreshold = data.data.alertThreshold;
          alertConsecutivePoints = data.data.alertConsecutivePoints;
          syncConfigForm();
          updateStatus();
        } else {
          alert(data.message);
        }
      } catch (e) { alert('保存失败: ' + e.message); }
    });

    document.getElementById('webhookForm').addEventListener('submit', async (e) => {
      e.preventDefault();
      const form = e.target;
      const fd = new FormData(form);
      try {
        const res = await fetch('/api/webhooks', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: fd.get('name') || undefined, type: fd.get('type'), url: fd.get('url'), secret: fd.get('secret') || undefined })
        });
        const data = await res.json();
        if (data.code === 0) { form.reset(); loadWebhooks(); } else { alert(data.message); }
      } catch (e) { alert('添加失败: ' + e.message); }
    });

    async function testWebhook(id) {
      try {
        const res = await fetch('/api/webhooks/' + id + '/test', { method: 'POST' });
        const data = await res.json();
        alert(data.code === 0 ? '测试消息已发送' : data.message);
        loadWebhooks();
      } catch (e) { alert('测试失败: ' + e.message); }
    }

    async function deleteWebhook(id) {
      if (!confirm('确定要删除这个 Webhook 吗？')) return;
      try {
        const res = await fetch('/api/webhooks/' + id, { method: 'DELETE' });
        const data = await res.json();
        if (data.code === 0) { loadWebhooks(); } else { alert(data.message); }
      } catch (e) { alert('删除失败: ' + e.message); }
    }

    async function refresh() {
      const btn = document.getElementById('refreshBtn');
      btn.classList.add('loading');
      await Promise.all([loadConfig(), updateStatus(), loadWebhooks(), loadAlerts()]);
      btn.classList.remove('loading');
    }

    document.getElementById('refreshBtn').addEventListener('click', refresh);
    function escapeHtml(t) { const d = document.createElement('div'); d.textContent = t; return d.innerHTML; }
    function webhookTypeLabel(t) { return t === 'wecom' ? '企业微信机器人' : '通用 JSON'; }

    initChart();
    refresh();
    setInterval(refresh, 30000);
  </script>
</body>
</html>`

const apiDocsHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>API 文档 - 服务状态监控</title>
  <style>
    :root {
      --bg-primary: #0f172a;
      --bg-secondary: #1e293b;
      --bg-card: #334155;
      --text-primary: #f1f5f9;
      --text-secondary: #94a3b8;
      --border-color: #475569;
      --accent: #3b82f6;
    }
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: var(--bg-primary);
      color: var(--text-primary);
      line-height: 1.6;
      padding: 2rem;
    }
    .container { max-width: 900px; margin: 0 auto; }
    header { margin-bottom: 3rem; }
    header h1 { font-size: 2rem; margin-bottom: 0.5rem; }
    header p { color: var(--text-secondary); }
    header a { color: var(--accent); text-decoration: none; }
    header a:hover { text-decoration: underline; }
    .section {
      background: var(--bg-secondary);
      border-radius: 12px;
      padding: 1.5rem;
      margin-bottom: 1.5rem;
      border: 1px solid var(--border-color);
    }
    .section h2 { font-size: 1.25rem; margin-bottom: 1rem; display: flex; align-items: center; gap: 0.5rem; }
    .method {
      display: inline-block;
      padding: 0.25rem 0.5rem;
      border-radius: 4px;
      font-size: 0.75rem;
      font-weight: 600;
      margin-right: 0.5rem;
    }
    .method.get { background: #22c55e; color: white; }
    .method.post { background: #3b82f6; color: white; }
    .method.delete { background: #ef4444; color: white; }
    .endpoint {
      font-family: monospace;
      background: var(--bg-card);
      padding: 0.5rem 0.75rem;
      border-radius: 6px;
      margin-bottom: 1rem;
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .endpoint code { flex: 1; }
    .description { color: var(--text-secondary); margin-bottom: 1rem; }
    h3 { font-size: 0.875rem; color: var(--text-secondary); margin: 1rem 0 0.5rem; text-transform: uppercase; letter-spacing: 0.05em; }
    pre {
      background: var(--bg-card);
      padding: 1rem;
      border-radius: 8px;
      overflow-x: auto;
      font-size: 0.875rem;
      line-height: 1.5;
    }
    code { font-family: 'SF Mono', Monaco, 'Consolas', monospace; }
    table { width: 100%; border-collapse: collapse; margin: 0.5rem 0; }
    th, td { text-align: left; padding: 0.75rem; border-bottom: 1px solid var(--border-color); }
    th { color: var(--text-secondary); font-weight: 500; font-size: 0.875rem; }
    td code { background: var(--bg-card); padding: 0.125rem 0.375rem; border-radius: 4px; font-size: 0.875rem; }
    .required { color: #ef4444; font-size: 0.75rem; }
    .optional { color: var(--text-secondary); font-size: 0.75rem; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>📚 API 文档</h1>
      <p>服务状态监控系统 API 接口说明 · <a href="/">返回状态页</a></p>
    </header>

    <div class="section">
      <h2>获取当前状态</h2>
      <div class="endpoint"><span class="method get">GET</span><code>/api/status</code></div>
      <p class="description">获取当前系统状态，包含 5/30/60 分钟窗口的错误率数据。</p>
      <h3>响应示例</h3>
      <pre><code>{
  "code": 0,
  "data": {
    "current": {
      "timestamp": "2026-07-06T08:30:00.000Z",
      "status5m": { "time_range": {...}, "error_ratio": 0.02 },
      "status30m": { ... },
      "status60m": { ... },
      "overall": { "status": "operational", "message": "服务正常运行" }
    },
    "config": { "alertThreshold": 0.05, "alertConsecutivePoints": 2, "checkInterval": 5 }
  }
}</code></pre>
    </div>

    <div class="section">
      <h2>获取状态历史</h2>
      <div class="endpoint"><span class="method get">GET</span><code>/api/status/history?limit=50</code></div>
      <p class="description">获取历史状态记录，用于绘制趋势图。</p>
    </div>

    <div class="section">
      <h2>获取告警历史</h2>
      <div class="endpoint"><span class="method get">GET</span><code>/api/alerts?limit=20</code></div>
    </div>

    <div class="section">
      <h2>告警规则配置</h2>
      <div class="endpoint"><span class="method get">GET</span><code>/api/config</code></div>
      <p class="description">获取当前告警阈值、连续采样点数和配置文件路径。</p>
      <div class="endpoint"><span class="method post">PUT</span><code>/api/config</code></div>
      <p class="description">更新告警规则，并写入本地配置文件。</p>
      <h3>请求体</h3>
      <pre><code>{
  "alertThreshold": 0.05,
  "alertConsecutivePoints": 2
}</code></pre>
    </div>

    <div class="section">
      <h2>注册 Webhook</h2>
      <div class="endpoint"><span class="method post">POST</span><code>/api/webhooks</code></div>
      <h3>请求体</h3>
      <table>
        <tr><th>字段</th><th>类型</th><th>说明</th></tr>
        <tr><td><code>url</code></td><td>string</td><td>Webhook URL <span class="required">必填</span></td></tr>
        <tr><td><code>name</code></td><td>string</td><td>显示名称 <span class="optional">可选</span></td></tr>
        <tr><td><code>type</code></td><td>string</td><td><code>generic</code> 或 <code>wecom</code>，企业微信 URL 可自动识别 <span class="optional">可选</span></td></tr>
        <tr><td><code>secret</code></td><td>string</td><td>验证密钥 <span class="optional">可选</span></td></tr>
      </table>
      <h3>示例</h3>
      <pre><code>curl -X POST https://api.fenno.ai/api/webhooks \
  -H "Content-Type: application/json" \
  -d '{"url": "https://your-server.com/webhook", "name": "主告警", "type": "generic", "secret": "xxx"}'</code></pre>
      <h3>企业微信机器人示例</h3>
      <pre><code>curl -X POST http://localhost:3000/api/webhooks \
  -H "Content-Type: application/json" \
  -d '{"url": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=YOUR_KEY", "name": "企业微信群告警", "type": "wecom"}'</code></pre>
    </div>

    <div class="section">
      <h2>其他 Webhook 接口</h2>
      <div class="endpoint"><span class="method get">GET</span><code>/api/webhooks</code></div>
      <p class="description">获取已注册的 Webhook 列表</p>
      <div class="endpoint"><span class="method delete">DELETE</span><code>/api/webhooks/:id</code></div>
      <p class="description">删除 Webhook</p>
      <div class="endpoint"><span class="method post">POST</span><code>/api/webhooks/:id/test</code></div>
      <p class="description">发送测试告警</p>
    </div>

    <div class="section">
      <h2>Webhook 推送格式</h2>
      <h3>告警推送</h3>
      <pre><code>{
  "type": "alert",
  "message": "服务可用性告警: 连续 2 个采样点错误率高于 5.00%，当前错误率 15.00%",
  "timestamp": "2026-07-06T08:30:00Z",
  "data": { "error_ratio": 0.15, "threshold": 0.05, "consecutive_points": 2, "status": "degraded" }
}</code></pre>
      <h3>恢复通知</h3>
      <pre><code>{
  "type": "recovery",
  "message": "服务已恢复正常",
  "timestamp": "2026-07-06T09:00:00Z",
  "data": { "error_ratio": 0.01, "status": "operational" }
}</code></pre>
    </div>
  </div>
</body>
</html>`
