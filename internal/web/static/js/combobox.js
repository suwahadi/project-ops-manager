(function () {
  "use strict";

  var SVG_NS = "http://www.w3.org/2000/svg";
  var counter = 0;

  function svgIcon(path, size) {
    var svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "2");
    svg.setAttribute("stroke-linecap", "round");
    svg.setAttribute("stroke-linejoin", "round");
    svg.setAttribute("aria-hidden", "true");
    svg.setAttribute("class", size + " shrink-0");
    var p = document.createElementNS(SVG_NS, "path");
    p.setAttribute("d", path);
    svg.appendChild(p);
    return svg;
  }

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) {
      node.className = className;
    }
    if (text) {
      node.textContent = text;
    }
    return node;
  }

  function Combobox(select) {
    this.select = select;
    this.multiple = select.multiple;
    this.inline = select.getAttribute("data-combobox") === "inline";
    this.searchable = this.multiple || this.inline || select.options.length > 8;
    this.typed = "";
    this.typedTimer = null;
    this.items = [];
    this.visible = [];
    this.active = -1;
    this.id = (select.id || "combo") + "-combo-" + (++counter);
    this.build();
    this.render();
  }

  Combobox.prototype.build = function () {
    var self = this;
    var select = this.select;
    Array.prototype.forEach.call(select.options, function (opt) {
      self.items.push({ option: opt, label: opt.textContent.trim(), hint: opt.getAttribute("data-hint") || "", empty: opt.value === "" });
    });

    this.root = el("div", "combo");
    select.parentNode.insertBefore(this.root, select);
    this.root.appendChild(select);
    select.hidden = true;
    select.tabIndex = -1;

    if (this.multiple) {
      this.chips = el("ul", "combo-chips");
      this.chips.hidden = true;
      this.root.appendChild(this.chips);
    }

    if (!this.inline) {
      this.trigger = el("button", "input combo-trigger");
      this.trigger.type = "button";
      this.trigger.id = this.id;
      this.trigger.setAttribute("aria-haspopup", "listbox");
      this.trigger.setAttribute("aria-expanded", "false");
      if (select.classList.contains("input-error")) {
        this.trigger.classList.add("input-error");
      }
      this.triggerText = el("span", "min-w-0 flex-1 truncate text-left");
      this.trigger.appendChild(this.triggerText);
      this.trigger.appendChild(svgIcon("m6 9 6 6 6-6", "h-4 w-4 text-slate-400"));
      this.root.appendChild(this.trigger);
      if (select.id) {
        var label = document.querySelector('label[for="' + select.id + '"]');
        if (label) {
          label.htmlFor = this.id;
        }
      }
    }

    this.panel = el("div", this.inline ? "combo-panel-inline" : "combo-panel");
    var searchWrap = el("div", this.searchable ? "relative" : "sr-only");
    searchWrap.appendChild(svgIcon("m21 21-6-6m2-5a7 7 0 1 1-14 0 7 7 0 0 1 14 0z", "pointer-events-none absolute left-3 top-3 h-4 w-4 text-slate-400"));
    this.search = el("input", "input pl-9");
    this.search.type = "search";
    this.search.autocomplete = "off";
    this.search.placeholder = "Search";
    this.search.setAttribute("aria-label", "Search");
    this.search.setAttribute("aria-controls", this.id + "-list");
    if (this.inline && select.id) {
      this.search.id = this.id;
      var inlineLabel = document.querySelector('label[for="' + select.id + '"]');
      if (inlineLabel) {
        inlineLabel.htmlFor = this.id;
      }
    }
    searchWrap.appendChild(this.search);
    this.panel.appendChild(searchWrap);
    this.list = el("ul", this.searchable ? "combo-list mt-2" : "combo-list");
    this.list.id = this.id + "-list";
    this.list.setAttribute("role", "listbox");
    if (this.multiple) {
      this.list.setAttribute("aria-multiselectable", "true");
    }
    this.panel.appendChild(this.list);
    this.emptyNote = el("p", "combo-empty", "No matches.");
    this.emptyNote.hidden = true;
    this.panel.appendChild(this.emptyNote);
    if (!this.inline) {
      this.panel.hidden = true;
    }
    this.root.appendChild(this.panel);

    if (this.trigger) {
      this.trigger.addEventListener("click", function () {
        self.toggle();
      });
      this.trigger.addEventListener("keydown", function (e) {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault();
          self.open();
        }
      });
    }
    this.search.addEventListener("input", function () {
      if (self.searchable) {
        self.filter();
      } else {
        self.typeahead();
      }
    });
    this.search.addEventListener("keydown", function (e) {
      self.onKey(e);
    });
    this.list.addEventListener("mousedown", function (e) {
      e.preventDefault();
    });
    this.list.addEventListener("click", function (e) {
      var li = e.target instanceof Element && e.target.closest("[data-index]");
      if (li) {
        self.choose(parseInt(li.getAttribute("data-index"), 10));
      }
    });
    if (this.chips) {
      this.chips.addEventListener("click", function (e) {
        var btn = e.target instanceof Element && e.target.closest("[data-remove]");
        if (btn) {
          var item = self.items[parseInt(btn.getAttribute("data-remove"), 10)];
          item.option.selected = false;
          self.changed();
        }
      });
    }
    this.filter();
  };

  Combobox.prototype.isOpen = function () {
    return this.inline || !this.panel.hidden;
  };

  Combobox.prototype.open = function () {
    if (this.inline || this.isOpen()) {
      return;
    }
    Combobox.closeAll(this);
    this.panel.hidden = false;
    this.trigger.setAttribute("aria-expanded", "true");
    this.search.value = "";
    this.filter();
    var selected = this.select.selectedIndex;
    var idx = this.visible.indexOf(selected);
    this.setActive(idx >= 0 ? idx : 0);
    this.search.focus();
  };

  Combobox.prototype.close = function (focusTrigger) {
    if (this.inline || !this.isOpen()) {
      return;
    }
    this.panel.hidden = true;
    this.trigger.setAttribute("aria-expanded", "false");
    if (focusTrigger) {
      this.trigger.focus();
    }
  };

  Combobox.prototype.toggle = function () {
    if (this.isOpen()) {
      this.close(true);
    } else {
      this.open();
    }
  };

  Combobox.prototype.filter = function () {
    var q = this.search.value.trim().toLowerCase();
    var self = this;
    this.visible = [];
    this.items.forEach(function (item, i) {
      if (self.multiple && item.empty) {
        return;
      }
      if (!q || (item.label + " " + item.hint).toLowerCase().indexOf(q) !== -1) {
        self.visible.push(i);
      }
    });
    this.renderList();
    this.setActive(this.visible.length ? 0 : -1);
  };

  Combobox.prototype.typeahead = function () {
    var self = this;
    var q = this.search.value.trim().toLowerCase();
    clearTimeout(this.typedTimer);
    this.typedTimer = setTimeout(function () {
      self.search.value = "";
    }, 700);
    if (!q) {
      return;
    }
    for (var pos = 0; pos < this.visible.length; pos++) {
      if (this.items[this.visible[pos]].label.toLowerCase().indexOf(q) === 0) {
        this.setActive(pos);
        return;
      }
    }
  };

  Combobox.prototype.renderList = function () {
    var self = this;
    this.list.textContent = "";
    this.visible.forEach(function (i) {
      var item = self.items[i];
      var li = el("li", "combo-option");
      li.id = self.id + "-opt-" + i;
      li.setAttribute("role", "option");
      li.setAttribute("data-index", String(i));
      li.setAttribute("aria-selected", item.option.selected ? "true" : "false");
      var check = svgIcon("M5 13l4 4L19 7", "combo-check h-4 w-4");
      li.appendChild(check);
      var text = el("span", "min-w-0 flex-1");
      text.appendChild(el("span", "block truncate", item.label));
      if (item.hint) {
        text.appendChild(el("span", "block truncate text-xs text-slate-500 dark:text-slate-400", item.hint));
      }
      li.appendChild(text);
      self.list.appendChild(li);
    });
    this.emptyNote.hidden = this.visible.length > 0;
  };

  Combobox.prototype.setActive = function (pos) {
    var prev = this.list.querySelector(".combo-option-active");
    if (prev) {
      prev.classList.remove("combo-option-active");
    }
    this.active = pos;
    if (pos < 0 || pos >= this.visible.length) {
      this.search.removeAttribute("aria-activedescendant");
      return;
    }
    var li = this.list.children[pos];
    li.classList.add("combo-option-active");
    this.search.setAttribute("aria-activedescendant", li.id);
    if (li.scrollIntoView) {
      li.scrollIntoView({ block: "nearest" });
    }
  };

  Combobox.prototype.onKey = function (e) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      this.setActive(Math.min(this.active + 1, this.visible.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      this.setActive(Math.max(this.active - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (this.active >= 0) {
        this.choose(this.visible[this.active]);
      }
    } else if (e.key === "Escape" && !this.inline) {
      e.preventDefault();
      e.stopPropagation();
      this.close(true);
    } else if (e.key === "Tab" && !this.inline) {
      this.close(false);
    }
  };

  Combobox.prototype.choose = function (index) {
    var item = this.items[index];
    if (this.multiple) {
      item.option.selected = !item.option.selected;
      var pos = this.active;
      this.changed();
      this.setActive(pos);
      return;
    }
    var changed = !item.option.selected;
    item.option.selected = true;
    this.render();
    this.close(true);
    if (changed) {
      this.select.dispatchEvent(new Event("change", { bubbles: true }));
    }
  };

  Combobox.prototype.changed = function () {
    this.render();
    this.select.dispatchEvent(new Event("change", { bubbles: true }));
  };

  Combobox.prototype.render = function () {
    var self = this;
    var selected = this.items.filter(function (item) {
      return item.option.selected;
    });
    Array.prototype.forEach.call(this.list.children, function (li) {
      var item = self.items[parseInt(li.getAttribute("data-index"), 10)];
      li.setAttribute("aria-selected", item.option.selected ? "true" : "false");
    });
    if (this.multiple) {
      this.chips.textContent = "";
      selected.forEach(function (item) {
        var chip = el("li", "combo-chip");
        chip.appendChild(el("span", "truncate", item.label));
        var remove = el("button", "combo-chip-remove");
        remove.type = "button";
        remove.setAttribute("data-remove", String(self.items.indexOf(item)));
        remove.setAttribute("aria-label", "Remove " + item.label);
        remove.appendChild(svgIcon("M6 18 18 6M6 6l12 12", "h-3.5 w-3.5"));
        chip.appendChild(remove);
        self.chips.appendChild(chip);
      });
      this.chips.hidden = selected.length === 0;
      if (this.triggerText) {
        var placeholder = this.select.getAttribute("data-placeholder") || "Choose";
        this.triggerText.textContent = selected.length ? selected.length + " selected" : placeholder;
        this.triggerText.classList.toggle("text-slate-400", selected.length === 0);
      }
      return;
    }
    var current = selected[0] || this.items[0];
    this.triggerText.textContent = current ? current.label : "";
    this.triggerText.classList.toggle("text-slate-400", !current || current.option.hasAttribute("data-placeholder"));
  };

  Combobox.instances = [];

  Combobox.closeAll = function (except) {
    Combobox.instances.forEach(function (c) {
      if (c !== except) {
        c.close(false);
      }
    });
  };

  function init(root) {
    var nodes = root.querySelectorAll ? root.querySelectorAll("select[data-combobox]") : [];
    Array.prototype.forEach.call(nodes, function (select) {
      if (select.hasAttribute("data-combobox-ready") || select.disabled) {
        return;
      }
      select.setAttribute("data-combobox-ready", "");
      Combobox.instances.push(new Combobox(select));
    });
    Combobox.instances = Combobox.instances.filter(function (c) {
      return document.body.contains(c.root);
    });
  }

  document.addEventListener("click", function (e) {
    var t = e.target;
    if (!(t instanceof Element)) {
      return;
    }
    Combobox.instances.forEach(function (c) {
      if (!c.root.contains(t)) {
        c.close(false);
      }
    });
  });

  document.addEventListener("DOMContentLoaded", function () {
    init(document);
  });

  document.addEventListener("htmx:load", function (e) {
    init(e.target);
  });
})();
