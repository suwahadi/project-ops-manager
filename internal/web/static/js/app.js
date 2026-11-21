(function () {
  "use strict";

  var themes = ["system", "light", "dark"];

  function setTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    document.cookie = "ops_theme=" + theme + "; path=/; max-age=31536000; samesite=lax" + (location.protocol === "https:" ? "; secure" : "");
    document.querySelectorAll("[data-theme-icon]").forEach(function (el) {
      el.hidden = el.getAttribute("data-theme-icon") !== theme;
    });
    document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
      el.setAttribute("aria-label", "Theme: " + theme);
      el.setAttribute("title", "Theme: " + theme);
    });
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
      var current = document.documentElement.getAttribute("data-theme") || "system";
      setTheme(themes[(themes.indexOf(current) + 1) % themes.length]);
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
    var tabBtn = t.closest("[data-tab-target]");
    if (tabBtn) {
      var group = tabBtn.closest("[data-tabs]");
      if (group) {
        group.querySelectorAll("[data-tab-target]").forEach(function (b) {
          var active = b === tabBtn;
          b.classList.toggle("tab-active", active);
          b.setAttribute("aria-selected", active ? "true" : "false");
        });
        group.querySelectorAll("[data-tab-panel]").forEach(function (p) {
          p.hidden = p.getAttribute("data-tab-panel") !== tabBtn.getAttribute("data-tab-target");
        });
      }
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
    root.querySelectorAll("[data-reveal-expires]").forEach(function (el) {
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
    root.querySelectorAll("dialog[data-open-on-load]").forEach(function (d) {
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
  });

  document.addEventListener("htmx:load", function (e) {
    initRevealTimers(e.target);
    initDialogs(e.target);
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
