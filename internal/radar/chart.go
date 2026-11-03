package radar

import "strings"

func chartTooltip(model Model, f Finding) string {
	change := "No previous placement available"
	if c := f.PlacementChange; c != nil {
		switch c.Kind {
		case "new":
			change = "New to this radar"
		case "unchanged":
			change = "No change in placement"
		default:
			previous := c.PreviousQuadrant
			if previous == "" {
				previous = "Unclassified"
			}
			change = "Previously: " + previous + " / " + c.PreviousRing
			if c.Kind == "moved-in" {
				change = "Moved inward. " + change
			} else if c.Kind == "moved-out" {
				change = "Moved outward. " + change
			}
		}
	}
	text := f.Name + "\n" + quadrant(f) + " / " + model.ring(f) + "\n" + status(f) + "\n" + change
	if f.GroupComponent != "" && f.GroupComponent != "." {
		text += "\nComponent: " + f.GroupComponent
	}
	if len(f.Members) > 1 {
		text += "\nIncludes: " + strings.Join(f.Members, ", ")
	}
	if f.Presence == "not detected" {
		text += "\nNot detected in this scan"
	}
	return text
}

const chartStyles = `
.radar-legend{display:flex;flex-wrap:wrap;justify-content:center;gap:12px 24px;color:var(--muted);font-size:14px}
.radar-legend span{display:flex;align-items:center;gap:8px}
.radar-legend svg{width:28px;height:28px;color:var(--accent);flex:none}
.chart .blip:focus-visible circle{stroke:var(--text);stroke-width:3}
#radar-tooltip{position:fixed;z-index:20;max-width:min(360px,calc(100vw - 16px));padding:10px 14px;border:1px solid var(--radar-border);border-radius:6px;color:var(--text);background:var(--control-bg);font-size:14px;line-height:1.5;white-space:pre-line;overflow-wrap:anywhere;pointer-events:none;box-shadow:0 4px 16px #0004}
@media print{#radar-tooltip{display:none}}
`

const chartScript = `<script>
document.addEventListener("DOMContentLoaded", () => {
  const tooltip = document.getElementById("radar-tooltip");
  let active;
  const hide = () => {
    tooltip.hidden = true;
    if (active) active.removeAttribute("aria-describedby");
    active = null;
  };
  const position = (point) => {
    const rect = point.getBoundingClientRect();
    if (rect.bottom < 0 || rect.top > window.innerHeight || rect.right < 0 || rect.left > window.innerWidth) {
      hide();
      return;
    }
    const width = tooltip.offsetWidth;
    const height = tooltip.offsetHeight;
    const left = Math.max(8, Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - width - 8));
    const above = rect.top - height - 10;
    const top = Math.max(8, Math.min(above >= 8 ? above : rect.bottom + 10, window.innerHeight - height - 8));
    tooltip.style.left = left + "px";
    tooltip.style.top = top + "px";
  };
  document.querySelectorAll(".chart .blip").forEach((point) => {
    const title = point.querySelector("title");
    if (title) title.remove();
    const show = () => {
      hide();
      active = point;
      tooltip.textContent = point.dataset.tooltip;
      tooltip.hidden = false;
      point.setAttribute("aria-describedby", "radar-tooltip");
      position(point);
    };
    point.addEventListener("mouseenter", show);
    point.addEventListener("focus", show);
    point.addEventListener("mouseleave", () => { if (document.activeElement !== point) hide(); });
    point.addEventListener("blur", hide);
    point.addEventListener("click", hide);
  });
  document.addEventListener("keydown", (event) => { if (event.key === "Escape") hide(); });
  document.addEventListener("scroll", () => {
    if (active && document.activeElement === active) position(active);
    else hide();
  }, true);
  window.addEventListener("resize", hide);
});
</script>`
