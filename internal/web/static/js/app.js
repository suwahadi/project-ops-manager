(function () {
  "use strict";

  function isDark() {
    var current = document.documentElement.getAttribute("data-theme");
    if (current === "dark") {
      return true;
    }
    if (current === "light") {
      return false;
    }
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  function setTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    document.cookie = "ops_theme=" + theme + "; path=/; max-age=31536000; samesite=lax" + (location.protocol === "https:" ? "; secure" : "");
  }

  function showToast(message, kind) {
    var root = document.getElementById("toasts");
    if (!root) {
      return;
    }
    var el = document.createElement("div");
    el.className = kind === "error" ? "toast-error" : "toast-success";
    el.setAttribute("role", "status");
    el.textContent = message;
    root.appendChild(el);
    scheduleDismiss(el);
  }

  function scheduleDismiss(el) {
    setTimeout(function () {
      el.remove();
    }, 4000);
  }

  function openDrawer(open) {
    var drawer = document.querySelector("[data-drawer]");
    var backdrop = document.getElementById("drawer-backdrop");
    if (!drawer) {
      return;
    }
    drawer.classList.toggle("hidden", !open);
    drawer.classList.toggle("flex", open);
    if (backdrop) {
      backdrop.hidden = !open;
    }
  }

  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text);
    }
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.className = "sr-only";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
    return Promise.resolve();
  }

  document.addEventListener("click", function (e) {
    var t = e.target;
    if (!(t instanceof Element)) {
      return;
    }
    var toggle = t.closest("[data-theme-toggle]");
    if (toggle) {
      setTheme(isDark() ? "light" : "dark");
      return;
    }
    if (t.closest("[data-drawer-open]")) {
      openDrawer(true);
      return;
    }
    if (t.closest("[data-drawer-close]")) {
      openDrawer(false);
      return;
    }
    var opener = t.closest("[data-confirm-open]");
    if (opener) {
      var dlg = document.getElementById(opener.getAttribute("data-confirm-open"));
      if (dlg && dlg.showModal) {
        dlg.showModal();
      }
      return;
    }
    var closer = t.closest("[data-dialog-close]");
    if (closer) {
      var d = closer.closest("dialog");
      if (d) {
        d.close();
        if (d.hasAttribute("data-remove-on-close")) {
          d.remove();
        }
      }
      return;
    }
    var copyBtn = t.closest("[data-copy-url]");
    if (copyBtn) {
      e.preventDefault();
      fetch(copyBtn.getAttribute("data-copy-url"), {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        credentials: "same-origin"
      }).then(function (r) {
        if (!r.ok) {
          throw new Error("copy failed");
        }
        return r.text();
      }).then(copyText).then(function () {
        showToast("Password copied.", "success");
      }).catch(function () {
        showToast("Could not copy the password.", "error");
      });
      return;
    }
    var collapse = t.closest("[data-collapse-toggle]");
    if (collapse) {
      var panel = document.getElementById(collapse.getAttribute("data-collapse-toggle"));
      if (panel) {
        var nowHidden = panel.classList.toggle("hidden");
        collapse.setAttribute("aria-expanded", nowHidden ? "false" : "true");
      }
      return;
    }
    var tabBtn = t.closest("[data-tab-target]");
    if (tabBtn) {
      var group = tabBtn.closest("[data-tabs]");
      if (group) {
        ownedBy(group, "[data-tab-target]").forEach(function (b) {
          var active = b === tabBtn;
          b.classList.toggle(b.classList.contains("segment") ? "segment-active" : "tab-active", active);
          b.setAttribute("aria-selected", active ? "true" : "false");
        });
        ownedBy(group, "[data-tab-panel]").forEach(function (p) {
          p.hidden = p.getAttribute("data-tab-panel") !== tabBtn.getAttribute("data-tab-target");
        });
      }
    }
  });

  function applyShowIf(root) {
    within(root, "[data-show-if]").forEach(function (el) {
      var spec = el.getAttribute("data-show-if").split(":");
      var src = document.getElementById(spec[0]);
      if (!src) {
        return;
      }
      var show = spec[1].split(",").indexOf(src.value) !== -1;
      el.hidden = !show;
      el.querySelectorAll("input, select, textarea").forEach(function (f) {
        f.disabled = !show;
      });
    });
  }

  document.addEventListener("change", function (e) {
    var t = e.target;
    if (t instanceof Element) {
      var auto = t.closest("form[data-autosubmit]");
      if (auto && auto.requestSubmit) {
        auto.requestSubmit();
        return;
      }
    }
    if (t instanceof Element && t.id && document.querySelector('[data-show-if^="' + t.id + ':"]')) {
      applyShowIf(document);
    }
  });

  document.addEventListener("input", function (e) {
    var t = e.target;
    if (t instanceof HTMLInputElement && t.hasAttribute("data-confirm-match")) {
      var form = t.closest("form");
      var submit = form && form.querySelector("[data-confirm-submit]");
      if (submit) {
        submit.disabled = t.value !== t.getAttribute("data-confirm-match");
      }
    }
  });

  function ownedBy(group, selector) {
    return Array.prototype.filter.call(group.querySelectorAll(selector), function (el) {
      var owner = el.parentElement && el.parentElement.closest("[data-tabs]");
      return owner === group;
    });
  }

  function within(root, selector) {
    var list = Array.prototype.slice.call(root.querySelectorAll ? root.querySelectorAll(selector) : []);
    if (root.matches && root.matches(selector)) {
      list.unshift(root);
    }
    return list;
  }

  function csrfToken() {
    var raw = document.body.getAttribute("hx-headers");
    if (!raw) {
      return "";
    }
    try {
      return JSON.parse(raw)["X-CSRF-Token"] || "";
    } catch (err) {
      return "";
    }
  }

  function initRevealTimers(root) {
    within(root, "[data-reveal-expires]").forEach(function (el) {
      if (el.getAttribute("data-reveal-armed")) {
        return;
      }
      el.setAttribute("data-reveal-armed", "1");
      var seconds = parseInt(el.getAttribute("data-reveal-expires"), 10) || 30;
      setTimeout(function () {
        var masked = document.createElement("span");
        masked.className = "font-mono";
        masked.textContent = "••••••••";
        el.replaceWith(masked);
      }, seconds * 1000);
    });
  }

  function initDialogs(root) {
    within(root, "dialog[data-open-on-load]").forEach(function (d) {
      d.removeAttribute("data-open-on-load");
      if (d.showModal && !d.open) {
        d.showModal();
      }
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    document.querySelectorAll("[data-toast]").forEach(scheduleDismiss);
    initRevealTimers(document);
    initDialogs(document);
    applyShowIf(document);
  });

  document.addEventListener("htmx:load", function (e) {
    initRevealTimers(e.target);
    initDialogs(e.target);
    applyShowIf(document);
  });

  document.addEventListener("showToast", function (e) {
    var d = e.detail || {};
    showToast(d.message || "", d.kind);
  });

  document.addEventListener("closeModal", function () {
    document.querySelectorAll("dialog[open]").forEach(function (d) {
      d.close();
      if (d.hasAttribute("data-remove-on-close")) {
        d.remove();
      }
    });
  });

  document.addEventListener("htmx:responseError", function (e) {
    var xhr = e.detail && e.detail.xhr;
    if (xhr && !xhr.getResponseHeader("HX-Trigger")) {
      showToast("Request failed (" + xhr.status + ").", "error");
    }
  });
})();
