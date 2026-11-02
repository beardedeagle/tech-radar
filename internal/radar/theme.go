package radar

const lightTheme = `
color-scheme:light;--bg:#fff;--text:#182c37;--muted:#526572;--border:#c5d8d7;
--notice-bg:#edf6f4;--accent:#00635e;--link:#205ba0;--focus:#087e9e;--warning:#894509;
--radar-outer:#f5f7f8;--radar-assess:#edf2f3;--radar-trial:#e4eeed;--radar-adopt:#d2e7e4;--radar-border:#b8c9ce;
--techniques:#00635e;--platforms:#2853a5;--tools:#965117;--languages:#7d4190;--control-bg:#f5f7f8;--point-outline:#fff;
`

const darkTheme = `
color-scheme:dark;--bg:#101820;--text:#dce6ee;--muted:#a9bccb;--border:#344a58;
--notice-bg:#172f33;--accent:#63d6c5;--link:#8dc5ff;--focus:#77d8ff;--warning:#f4bd78;
--radar-outer:#16212c;--radar-assess:#1c2a36;--radar-trial:#223641;--radar-adopt:#29464d;--radar-border:#56717e;
--techniques:#64d7c9;--platforms:#92b9ff;--tools:#edb77d;--languages:#d8a4eb;--control-bg:#20313e;--point-outline:#c8d8e1;
`

// System colors work without JavaScript; explicit choices are applied before
// the page is painted. Printing always uses the light palette.
const themeStyles = `:root{` + lightTheme + `}
[data-theme="dark"]{` + darkTheme + `}
@media screen and (prefers-color-scheme:dark){:root:not([data-theme="light"]){` + darkTheme + `}}
.theme-control{display:flex;justify-content:flex-end;align-items:center;gap:10px;color:var(--muted);font-size:14px}
.theme-control[hidden]{display:none}
.theme-control select{font:inherit;padding:6px 10px;border:1px solid var(--border);border-radius:6px;color:var(--text);background:var(--control-bg)}
.theme-control select:focus-visible{outline:3px solid var(--focus);outline-offset:3px}
@media print{:root,[data-theme]{` + lightTheme + `}.theme-control{display:none}}
`

const themeScript = `<script>
(() => {
  const root = document.documentElement;
  const key = "tech-radar-theme";
  const apply = (theme) => {
    if (theme === "light" || theme === "dark") root.dataset.theme = theme;
    else root.removeAttribute("data-theme");
  };
  try { apply(localStorage.getItem(key)); } catch (_) {}
  document.addEventListener("DOMContentLoaded", () => {
    const control = document.getElementById("theme-control");
    const select = document.getElementById("theme");
    select.value = root.dataset.theme || "system";
    control.hidden = false;
    select.addEventListener("change", () => {
      apply(select.value);
      try {
        if (select.value === "system") localStorage.removeItem(key);
        else localStorage.setItem(key, select.value);
      } catch (_) {}
    });
  });
})();
</script>`
