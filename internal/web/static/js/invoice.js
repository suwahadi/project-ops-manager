(function () {
  "use strict";

  var ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";

  function toCents(raw) {
    var s = String(raw || "").trim().replace(/,/g, "").replace(/%$/, "");
    if (!/^\d*(\.\d{0,2})?$/.test(s) || s === "" || s === ".") {
      return null;
    }
    var parts = s.split(".");
    var whole = parseInt(parts[0] || "0", 10);
    var frac = (parts[1] || "").padEnd(2, "0");
    return whole * 100 + parseInt(frac, 10);
  }

  function roundDiv(n, d) {
    return Math.round(n / d);
  }

  function group(n, sep) {
    return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, sep);
  }

  function format(cents, currency) {
    var whole = Math.floor(cents / 100);
    var frac = cents % 100;
    if (currency === "USD") {
      return "$" + group(whole, ",") + "." + String(frac).padStart(2, "0");
    }
    return "Rp " + group(whole, ".") + (frac ? "," + String(frac).padStart(2, "0") : "");
  }

  function generate(prefix) {
    var out = (prefix || "").toUpperCase();
    var limit = 256 - (256 % ALPHABET.length);
    var buf = new Uint8Array(1);
    var n = 0;
    while (n < 8) {
      window.crypto.getRandomValues(buf);
      if (buf[0] >= limit) {
        continue;
      }
      out += ALPHABET.charAt(buf[0] % ALPHABET.length);
      n++;
    }
    return out;
  }

  function InvoiceForm(form) {
    this.form = form;
    this.rows = form.querySelector("[data-invoice-rows]");
    this.template = form.querySelector("template[data-invoice-template]");
    this.currency = form.querySelector("[data-invoice-currency]");
    this.billTo = form.querySelector("[data-invoice-billto]");
    this.client = form.querySelector("[data-invoice-client]");
    this.lastClientName = this.clientName();
    this.bind();
    this.renumber();
    this.recalc();
  }

  InvoiceForm.prototype.clientName = function () {
    if (!this.client || !this.client.value) {
      return "";
    }
    var opt = this.client.options[this.client.selectedIndex];
    return opt ? opt.textContent.trim() : "";
  };

  InvoiceForm.prototype.bind = function () {
    var self = this;
    this.form.addEventListener("click", function (e) {
      var t = e.target instanceof Element ? e.target : null;
      if (!t) {
        return;
      }
      var gen = t.closest("[data-invoice-generate]");
      if (gen) {
        var input = self.form.querySelector("#number");
        input.value = generate(gen.getAttribute("data-prefix"));
        input.focus();
        return;
      }
      if (t.closest("[data-invoice-add]")) {
        self.addRow();
        return;
      }
      var row = t.closest("[data-invoice-row]");
      if (!row) {
        return;
      }
      if (t.closest("[data-invoice-remove]")) {
        if (self.rows.querySelectorAll("[data-invoice-row]").length > 1) {
          row.remove();
        } else {
          row.querySelectorAll("input, textarea").forEach(function (f) {
            f.value = f.hasAttribute("data-invoice-qty") ? "1" : f.hasAttribute("data-invoice-taxrate") ? "0" : "";
          });
        }
        self.renumber();
        self.recalc();
      } else if (t.closest("[data-invoice-up]") && row.previousElementSibling) {
        row.parentNode.insertBefore(row, row.previousElementSibling);
        self.renumber();
      } else if (t.closest("[data-invoice-down]") && row.nextElementSibling) {
        row.parentNode.insertBefore(row.nextElementSibling, row);
        self.renumber();
      }
    });
    this.form.addEventListener("input", function (e) {
      var t = e.target;
      if (t instanceof Element && t.matches("[data-invoice-qty], [data-invoice-rate], [data-invoice-taxrate]")) {
        self.recalc();
      }
    });
    this.form.addEventListener("change", function (e) {
      if (e.target === self.currency) {
        self.recalc();
      }
      if (e.target === self.client && self.billTo) {
        var name = self.clientName();
        if (name && (self.billTo.value.trim() === "" || self.billTo.value.trim() === self.lastClientName)) {
          self.billTo.value = name;
        }
        self.lastClientName = name;
      }
    });
  };

  InvoiceForm.prototype.addRow = function () {
    var frag = this.template.content.cloneNode(true);
    var row = frag.querySelector("[data-invoice-row]");
    this.rows.appendChild(frag);
    this.renumber();
    this.recalc();
    var title = row.querySelector("[data-invoice-title]");
    if (title) {
      title.focus();
    }
  };

  InvoiceForm.prototype.renumber = function () {
    var rows = this.rows.querySelectorAll("[data-invoice-row]");
    rows.forEach(function (row, i) {
      var n = row.querySelector("[data-invoice-row-number]");
      if (n) {
        n.textContent = String(i + 1);
      }
      row.querySelector("[data-invoice-up]").disabled = i === 0;
      row.querySelector("[data-invoice-down]").disabled = i === rows.length - 1;
    });
  };

  InvoiceForm.prototype.recalc = function () {
    var currency = this.currency ? this.currency.value : "IDR";
    var subtotal = 0;
    var tax = 0;
    var valid = true;
    this.rows.querySelectorAll("[data-invoice-row]").forEach(function (row) {
      var qtyRaw = row.querySelector("[data-invoice-qty]").value;
      var rateRaw = row.querySelector("[data-invoice-rate]").value;
      var taxRaw = row.querySelector("[data-invoice-taxrate]").value;
      var qty = toCents(qtyRaw === "" ? "1" : qtyRaw);
      var rate = toCents(rateRaw);
      var pct = toCents(taxRaw === "" ? "0" : taxRaw);
      var out = row.querySelector("[data-invoice-amount]");
      if (qty === null || rate === null || pct === null) {
        out.textContent = "—";
        if (rateRaw !== "") {
          valid = false;
        }
        return;
      }
      var amount = roundDiv(qty * rate, 100);
      subtotal += amount;
      tax += roundDiv(amount * pct, 10000);
      out.textContent = format(amount, currency);
    });
    var set = function (sel, v) {
      var el = this.form.querySelector(sel);
      if (el) {
        el.textContent = valid ? format(v, currency) : "—";
      }
    }.bind(this);
    set("[data-invoice-subtotal]", subtotal);
    set("[data-invoice-tax]", tax);
    set("[data-invoice-total]", subtotal + tax);
  };

  function init(root) {
    var forms = root.querySelectorAll ? root.querySelectorAll("form[data-invoice-form]") : [];
    forms.forEach(function (form) {
      if (form.hasAttribute("data-invoice-ready")) {
        return;
      }
      form.setAttribute("data-invoice-ready", "");
      new InvoiceForm(form);
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    init(document);
  });
})();
