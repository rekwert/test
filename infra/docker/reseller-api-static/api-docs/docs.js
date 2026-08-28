(function () {
  const METHOD_ORDER = { get: 0, post: 1, patch: 2, delete: 3 };
  const METHOD_LABEL = { get: "GET", post: "POST", patch: "PATCH", delete: "DELETE" };

  const tagTitles = {
    account: "Аккаунт",
    billing: "Биллинг",
    catalog: "Каталог",
    orders: "Заказы",
    instances: "Серверы",
    power: "Питание",
  };

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function slug(path, method) {
    return (method + "-" + path).replace(/[^a-z0-9]+/gi, "-").replace(/^-|-$/g, "").toLowerCase();
  }

  function buildCurl(path, method, needsAuth) {
    const url = "https://api.cloud-hustle.com/api/v1" + path.replace(/\{[^}]+\}/g, "UUID");
    const auth = needsAuth ? `\n  -H "X-Api-Key: ch_live_ВАШ_КЛЮЧ"` : "";
    if (method === "get") {
      return `curl -sS "${url}"${auth}`;
    }
    if (method === "delete") {
      return `curl -sS -X DELETE "${url}"${auth}`;
    }
    return `curl -sS -X POST "${url}" \\\n  -H "Content-Type: application/json"${auth} \\\n  -d '{"plan_id":"...","os_template_id":"..."}'`;
  }

  function renderAuth(spec) {
    const scheme = spec.components?.securitySchemes?.ApiKeyAuth;
    const desc = scheme?.description || "Передайте ключ в заголовке X-Api-Key.";
    return `
      <section class="auth-card" id="auth">
        <h2 class="section-title">Авторизация</h2>
        <p>${esc(desc)}</p>
        <pre class="code-block">curl -sS https://api.cloud-hustle.com/api/v1/auth/me \\
  -H "X-Api-Key: ch_live_ВАШ_КЛЮЧ"</pre>
        <div class="pill-row">
          <span class="pill">billing.read — баланс</span>
          <span class="pill">vps.read — список серверов</span>
          <span class="pill">vps.write — заказ VPS</span>
          <span class="pill">vps.manage — питание и удаление</span>
        </div>
      </section>`;
  }

  function renderEndpoint(path, method, op, spec) {
    const id = slug(path, method);
    const needsAuth = !(op.security && op.security.length === 0);
    const params = op.parameters || [];
    const responses = op.responses || {};
    const responseHtml = Object.entries(responses)
      .map(([code, r]) => `<span class="response-chip"><em>${esc(code)}</em>${esc(r.description || "")}</span>`)
      .join("");

    const paramsHtml = params.length
      ? `<ul class="params-list">${params
          .map((p) => {
            const name = p.name || p.$ref?.split("/").pop();
            const desc = p.description || "";
            const where = p.in === "query" ? "query" : p.in === "path" ? "path" : p.in;
            return `<li><code>${esc(name)}</code> (${esc(where)}) — ${esc(desc)}</li>`;
          })
          .join("")}</ul>`
      : "";

    return `
      <article class="endpoint-card" id="${id}">
        <div class="endpoint-head">
          <span class="method method-${method}">${METHOD_LABEL[method]}</span>
          <code class="path">${esc(path)}</code>
        </div>
        <h4 class="endpoint-title">${esc(op.summary || path)}</h4>
        <p class="endpoint-desc">${esc(op.description || "")}</p>
        ${paramsHtml}
        <div class="meta-grid">
          <div class="meta-item"><strong>Доступ</strong>${needsAuth ? "Требуется API-ключ" : "Публичный"}</div>
          <div class="meta-item"><strong>Пример</strong><code>${METHOD_LABEL[method]}</code></div>
        </div>
        <pre class="code-block">${esc(buildCurl(path, method, needsAuth))}</pre>
        <div class="responses">${responseHtml}</div>
      </article>`;
  }

  function groupEndpoints(spec) {
    const groups = new Map();
    for (const [path, item] of Object.entries(spec.paths || {})) {
      for (const method of Object.keys(item)) {
        if (!METHOD_ORDER.hasOwnProperty(method)) continue;
        const op = item[method];
        const tags = op.tags?.length ? op.tags : ["other"];
        for (const tag of tags) {
          if (!groups.has(tag)) groups.set(tag, []);
          groups.get(tag).push({ path, method, op });
        }
      }
    }
    for (const list of groups.values()) {
      list.sort((a, b) => METHOD_ORDER[a.method] - METHOD_ORDER[b.method] || a.path.localeCompare(b.path));
    }
    return groups;
  }

  function renderNav(groups, spec) {
    const tagMeta = new Map((spec.tags || []).map((t) => [t.name, t]));
    let html = "";
    for (const [tag, items] of groups) {
      const title = tagMeta.get(tag)?.["x-title"] || tagTitles[tag] || tag;
      html += `<div class="nav-group"><div class="nav-group-title">${esc(title)}</div>`;
      for (const { path, method, op } of items) {
        const id = slug(path, method);
        html += `<a href="#${id}">${esc(METHOD_LABEL[method])} ${esc(op.summary || path)}</a>`;
      }
      html += "</div>";
    }
    return html;
  }

  function renderContent(groups, spec) {
    const tagMeta = new Map((spec.tags || []).map((t) => [t.name, t]));
    let html = renderAuth(spec);
    html += `<section class="intro-card"><h2 class="section-title">${esc(spec.info?.title || "API")}</h2><p>${esc(spec.info?.description || "")}</p></section>`;

    for (const [tag, items] of groups) {
      const meta = tagMeta.get(tag);
      const title = meta?.["x-title"] || tagTitles[tag] || tag;
      const desc = meta?.description || "";
      html += `<section class="tag-section" id="tag-${esc(tag)}"><div class="tag-heading"><h3>${esc(title)}</h3><span>${esc(desc)}</span></div>`;
      for (const item of items) {
        html += renderEndpoint(item.path, item.method, item.op, spec);
      }
      html += "</section>";
    }
    return html;
  }

  async function init() {
    const sidebar = document.getElementById("sidebar");
    const content = document.getElementById("content");
    try {
      const res = await fetch("/api-docs/openapi.json");
      if (!res.ok) throw new Error("Не удалось загрузить спецификацию");
      const spec = await res.json();
      const groups = groupEndpoints(spec);
      sidebar.innerHTML = `<div class="sidebar-card"><h2>Методы</h2>${renderNav(groups, spec)}</div>`;
      content.innerHTML = renderContent(groups, spec);

      const links = sidebar.querySelectorAll("a[href^='#']");
      const observer = new IntersectionObserver(
        (entries) => {
          entries.forEach((entry) => {
            if (!entry.isIntersecting) return;
            const id = entry.target.id;
            links.forEach((a) => a.classList.toggle("active", a.getAttribute("href") === "#" + id));
          });
        },
        { rootMargin: "-30% 0px -55% 0px", threshold: 0 }
      );
      content.querySelectorAll(".endpoint-card").forEach((el) => observer.observe(el));
    } catch (e) {
      content.innerHTML = `<div class="error">${esc(e.message || "Ошибка загрузки")}</div>`;
    }
  }

  document.addEventListener("DOMContentLoaded", init);
})();