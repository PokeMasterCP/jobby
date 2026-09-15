(() => {
  const parseNumber = (value) => {
    const n = Number.parseFloat(value);
    return Number.isFinite(n) ? n : null;
  };

  const statusRank = {
    applied: 1,
    in_contact: 2,
    accepted: 3,
    rejected_after_contact: 4,
    rejected_no_contact: 5,
  };

  const rowValue = (row, key, type) => {
    if (key === "salary") {
      const min = parseNumber(row.dataset.salaryMin);
      const max = parseNumber(row.dataset.salaryMax);
      if (min === null && max === null) {
        return null;
      }
      if (min === null) {
        return max;
      }
      if (max === null) {
        return min;
      }
      return (min + max) / 2;
    }

    const raw = (row.dataset[key] ?? "").trim();
    if (type === "number") {
      return parseNumber(raw);
    }
    if (type === "status") {
      return statusRank[raw] ?? 99;
    }
    if (type === "date") {
      if (!raw) {
        return null;
      }
      const normalized = raw.replace(/^Updated\s+/i, "");
      const timestamp = Date.parse(normalized);
      return Number.isFinite(timestamp) ? timestamp : null;
    }
    return raw.toLowerCase();
  };

  const currentURL = () => new URL(window.location.href);

  const bindTable = (root) => {
    const list = root.querySelector("[data-table-list]");
    if (!list) {
      return;
    }

    const items = Array.from(list.children).filter((child) => child.querySelector("[data-table-row]"));
    const rows = items.map((item) => item.querySelector("[data-table-row]"));
    const searchInput = root.querySelector("[data-table-search]");
    const filterInputs = Array.from(root.querySelectorAll("[data-table-filter]"));
    const sortButtons = Array.from(root.querySelectorAll("[data-sort]"));
    const empty = root.querySelector("[data-table-empty]");
    const count = root.querySelector("[data-visible-count]");
    const sortLabel = root.querySelector("[data-sort-label]");
    const clearButton = root.querySelector("[data-clear-client-filters]");
    const params = currentURL().searchParams;

    let sortKey = params.get("sort") || "";
    let sortDir = params.get("dir") === "asc" || params.get("dir") === "desc" ? params.get("dir") : "asc";
    let search = params.get("q") || "";

    if (searchInput && search) {
      searchInput.value = search;
    }

    filterInputs.forEach((input) => {
      const paramName = input.dataset.filterParam || input.dataset.tableFilter;
      const value = params.get(paramName);
      if (value) {
        input.value = value;
      }
    });

    const syncHiddenFields = (url) => {
      document.querySelectorAll("[data-persist]").forEach((field) => {
        field.value = url.searchParams.get(field.dataset.persist) || "";
      });
    };

    const persist = () => {
      const url = currentURL();
      const assign = (key, value) => {
        if (value) {
          url.searchParams.set(key, value);
        } else {
          url.searchParams.delete(key);
        }
      };

      assign("q", search.trim());
      assign("sort", sortKey);
      assign("dir", sortKey ? sortDir : "");
      filterInputs.forEach((input) => {
        const paramName = input.dataset.filterParam || input.dataset.tableFilter;
        assign(paramName, input.value);
      });
      syncHiddenFields(url);
      history.replaceState(null, "", url);
      updateClearState();
    };

    const updateClearState = () => {
      if (!clearButton) {
        return;
      }
      const url = currentURL();
      const onApplications = url.pathname === "/applications";
      const hasClientState = Boolean(
        (searchInput && searchInput.value.trim()) ||
        filterInputs.some((input) => input.value) ||
        (onApplications && (url.searchParams.get("status") || url.searchParams.get("income") || url.searchParams.get("organization"))) ||
        url.searchParams.get("q") ||
        url.searchParams.get("sort") ||
        url.searchParams.get("location") ||
        url.searchParams.get("portal") ||
        url.searchParams.get("open")
      );
      clearButton.hidden = !hasClientState;
    };

    const apply = () => {
      const query = search.trim().toLowerCase();
      const decorated = rows.map((row, index) => ({ row, item: items[index], index }));

      if (sortKey) {
        const type = sortButtons.find((button) => button.dataset.sort === sortKey)?.dataset.sortType || "string";
        decorated.sort((a, b) => {
          const av = rowValue(a.row, sortKey, type);
          const bv = rowValue(b.row, sortKey, type);
          const aNull = av === null || av === "";
          const bNull = bv === null || bv === "";
          if (aNull && bNull) {
            return a.index - b.index;
          }
          if (aNull) {
            return 1;
          }
          if (bNull) {
            return -1;
          }
          let cmp = 0;
          if (type === "number" || type === "date" || type === "status") {
            cmp = av - bv;
          } else {
            cmp = String(av).localeCompare(String(bv), undefined, { numeric: true, sensitivity: "base" });
          }
          if (cmp === 0) {
            return a.index - b.index;
          }
          return sortDir === "asc" ? cmp : -cmp;
        });
        decorated.forEach(({ item }) => list.appendChild(item));
      }

      let visible = 0;
      rows.forEach((row, index) => {
        const haystack = (row.dataset.search || row.textContent || "").toLowerCase();
        const matchesSearch = !query || haystack.includes(query);
        const matchesFilters = filterInputs.every((input) => {
          if (!input.value) {
            return true;
          }
          return (row.dataset[input.dataset.tableFilter] || "") === input.value;
        });
        const show = matchesSearch && matchesFilters;
        items[index].hidden = !show;
        if (show) {
          visible += 1;
        }
      });

      if (empty) {
        empty.hidden = visible !== 0;
      }
      if (count) {
        count.textContent = String(visible);
      }

      sortButtons.forEach((button) => {
        if (button.dataset.sort === sortKey) {
          button.setAttribute("aria-sort", sortDir === "asc" ? "ascending" : "descending");
        } else {
          button.removeAttribute("aria-sort");
        }
      });

      if (sortLabel) {
        if (sortKey) {
          const button = sortButtons.find((candidate) => candidate.dataset.sort === sortKey);
          const label = button?.dataset.sortLabel || button?.textContent.trim() || sortKey;
          const direction = sortDir === "asc" ? "ascending" : "descending";
          sortLabel.textContent = `Sorted by ${label}, ${direction}`;
        } else {
          sortLabel.textContent = "Original order";
        }
      }
    };

    searchInput?.addEventListener("input", () => {
      search = searchInput.value;
      persist();
      apply();
    });

    filterInputs.forEach((input) => {
      input.addEventListener("change", () => {
        persist();
        apply();
      });
    });

    sortButtons.forEach((button) => {
      button.addEventListener("click", () => {
        const key = button.dataset.sort;
        if (sortKey === key) {
          sortDir = sortDir === "asc" ? "desc" : "asc";
        } else {
          sortKey = key;
          sortDir = button.dataset.sortDefault || "asc";
        }
        persist();
        apply();
      });
    });

    syncHiddenFields(currentURL());
    apply();
    updateClearState();
  };

  document.querySelectorAll("[data-sortable-table]").forEach(bindTable);

  document.querySelectorAll("[data-auto-submit]").forEach((form) => {
    form.querySelectorAll("select").forEach((field) => {
      if (field.hasAttribute("data-table-filter")) {
        return;
      }
      field.addEventListener("change", () => form.requestSubmit());
    });
  });

  document.querySelectorAll("[data-status-chip]").forEach((chip) => {
    chip.addEventListener("click", () => {
      const form = chip.closest("form");
      const field = form?.querySelector("[data-status-field]");
      if (!form || !field) {
        return;
      }
      field.value = chip.dataset.statusChip;
      form.requestSubmit();
    });
  });
})();
