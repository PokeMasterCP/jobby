(() => {
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

  // Toasts

  const toastRegion = $("[data-toasts]");
  const toast = (message, tone = "success") => {
    if (!toastRegion || !message) return;
    const element = document.createElement("div");
    element.className = `toast toast-${tone}`;
    element.innerHTML = `<svg class="icon" aria-hidden="true"><use href="#i-${tone === "error" ? "alert" : "check"}"></use></svg>`;
    const text = document.createElement("span");
    text.textContent = message;
    element.append(text);
    toastRegion.append(element);
    // Re-showing the popover keeps toasts above any open dialog in the top layer.
    if (toastRegion.showPopover) {
      if (toastRegion.matches(":popover-open")) toastRegion.hidePopover();
      toastRegion.showPopover();
    }
    setTimeout(() => {
      element.classList.add("is-leaving");
      setTimeout(() => element.remove(), 300);
    }, 3200);
  };

  // A flash message survives the full-page reload that follows a regular form submission.
  const FLASH_KEY = "jobby.flash";
  const takeFlash = () => {
    try {
      const message = sessionStorage.getItem(FLASH_KEY);
      sessionStorage.removeItem(FLASH_KEY);
      return message;
    } catch {
      return null;
    }
  };
  const setFlash = (message) => {
    try {
      if (message) sessionStorage.setItem(FLASH_KEY, message);
    } catch {
      // The toast is only a convenience.
    }
  };

  // Dialogs

  const openDialog = (dialog) => {
    if (dialog && !dialog.open) dialog.showModal();
  };

  let pointerStartedOnBackdrop = false;
  document.addEventListener("pointerdown", (event) => {
    pointerStartedOnBackdrop = event.target instanceof HTMLDialogElement;
  });

  const clearFormErrors = (form) => {
    if (!form) return;
    form.querySelectorAll(".form-alert[role='alert'], .field-error").forEach((error) => {
      if (error.hasAttribute("data-settings-error")) {
        error.hidden = true;
      } else {
        error.remove();
      }
    });
    form.querySelectorAll('[aria-invalid="true"]').forEach((field) => {
      field.removeAttribute("aria-invalid");
      field.removeAttribute("aria-describedby");
    });
  };

  const focusFirstError = (root) => {
    const field = root.querySelector('[aria-invalid="true"]');
    (field?.matches("input, select, textarea") ? field : field?.querySelector("input"))?.focus();
  };

  // Salary privacy

  const applySalaryPrivacy = (hidden) => {
    document.documentElement.dataset.salaryPrivacy = String(hidden);
    $$("input[data-salary-privacy]").forEach((toggle) => {
      toggle.checked = hidden;
    });
    $$("[data-toggle-salaries]").forEach((button) => {
      button.setAttribute("aria-pressed", String(hidden));
      const label = hidden ? "Show salaries" : "Hide salaries";
      if (button.hasAttribute("aria-label")) button.setAttribute("aria-label", label);
      const text = $("[data-salary-toggle-label]", button);
      if (text) text.textContent = label;
    });
    $$(".salary-input input").forEach((input) => {
      if (!input.hasAttribute("data-placeholder")) {
        input.dataset.placeholder = input.getAttribute("placeholder") || "";
      }
      input.type = hidden ? "password" : "number";
      input.placeholder = hidden ? "Hidden" : input.dataset.placeholder;
    });
  };

  const setSalaryPrivacy = (hidden) => {
    applySalaryPrivacy(hidden);
    try {
      localStorage.setItem("jobby.hideSalaries", String(hidden));
    } catch {
      // The preference still applies to this page.
    }
    toast(hidden ? "Salaries hidden" : "Salaries visible");
  };

  window.addEventListener("storage", (event) => {
    if (event.key === "jobby.hideSalaries" || event.key === null) {
      applySalaryPrivacy(event.newValue === "true");
    }
  });

  // Theme

  const THEME_KEY = "jobby.theme";
  const readThemePreference = () => {
    try {
      return localStorage.getItem(THEME_KEY) || "system";
    } catch {
      return "system";
    }
  };

  const setTheme = (preference) => {
    try {
      if (preference === "system") {
        localStorage.removeItem(THEME_KEY);
      } else {
        localStorage.setItem(THEME_KEY, preference);
      }
    } catch {
      // The choice still applies to this page.
    }
    const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    const dark = preference === "dark" || (preference === "system" && systemDark);
    document.documentElement.dataset.theme = dark ? "dark" : "light";
  };

  // New application dialog

  const createDialog = $("#create-dialog");
  const createForm = $("[data-create-form]");
  const organizationChoices = new Map(
    $$("#organization-choices option").map((option) => [option.value.toLowerCase(), option.dataset.careersUrl || ""]),
  );

  const updateCareersField = () => {
    if (!createForm) return;
    const name = createForm.elements.namedItem("organization_name").value.trim();
    const careersField = $("[data-careers-field]", createForm);
    const careersInput = createForm.elements.namedItem("careers_url");
    const known = $("[data-portal-known]", createForm);
    const savedURL = organizationChoices.get(name.toLowerCase());
    const hasError = careersInput.getAttribute("aria-invalid") === "true";

    // An organization that already has a portal keeps it; editing it happens on the Organizations page.
    const useSaved = Boolean(savedURL) && !hasError;
    careersField.hidden = useSaved;
    careersInput.disabled = useSaved;
    known.hidden = !useSaved;
    if (useSaved) {
      let host = savedURL;
      try {
        host = new URL(savedURL).hostname.replace(/^www\./, "");
      } catch {
        // Show the stored value as-is.
      }
      $("span", known).textContent = `Portal on file: ${host}`;
    } else if (organizationChoices.has(name.toLowerCase())) {
      known.hidden = false;
      $("span", known).textContent = "Existing organization, no portal saved yet";
    }
  };

  const localDate = () => {
    const now = new Date();
    const offset = now.getTimezoneOffset() * 60000;
    return new Date(now.getTime() - offset).toISOString().slice(0, 10);
  };

  const openCreate = () => {
    if (!createDialog) return;
    const date = $("[data-default-today]", createForm);
    if (date && !date.value) date.value = localDate();
    updateCareersField();
    openDialog(createDialog);
    (createForm.querySelector('[aria-invalid="true"]') || createForm.elements.namedItem("organization_name")).focus();
  };

  createForm?.elements.namedItem("organization_name").addEventListener("input", updateCareersField);

  // Application drawer

  const drawer = $("#application-drawer");
  const editForm = $("[data-edit-form]", drawer || document);
  let currentApplicationID = null;

  const field = (name) => $(`[data-field="${name}"]`, drawer);

  const setLink = (link, url) => {
    if (url) {
      link.href = url;
      link.hidden = false;
    } else {
      link.removeAttribute("href");
      link.hidden = true;
    }
  };

  const setDrawerActions = (id) => {
    $("[data-status-form]", drawer).action = `/applications/${id}/status`;
    $("[data-checked-form]", drawer).action = `/applications/${id}/checked`;
    $("[data-delete-form]", drawer).action = `/applications/${id}/delete`;
    if (editForm) editForm.action = `/applications/${id}`;
  };

  let currentApplication = null;

  const parseHistory = (value) => {
    try {
      return JSON.parse(value || "[]");
    } catch {
      return [];
    }
  };

  const renderHistory = (value) => {
    $$("[data-history-step]", drawer).forEach((step) => step.remove());
    const checkStep = field("check-step");
    parseHistory(value).forEach((change) => {
      const step = document.createElement("li");
      step.className = change.statusClass;
      step.dataset.historyStep = "";
      const what = document.createElement("span");
      what.className = "timeline-what";
      what.textContent = `Moved to ${change.label}`;
      const when = document.createElement("span");
      when.className = "timeline-when";
      const date = document.createElement("span");
      date.textContent = change.date;
      const ago = document.createElement("small");
      ago.textContent = change.ago;
      when.append(date, ago);
      step.append(what, when);
      checkStep.before(step);
    });
  };

  const fillDrawer = (data) => {
    currentApplication = { ...data };
    currentApplicationID = data.applicationId;
    const mark = field("mark");
    mark.className = `org-mark org-mark-large ${data.organizationMark}`;
    mark.textContent = data.organizationInitial;
    const organization = field("organization");
    organization.textContent = data.organizationName;
    organization.href = `/applications?organization=${data.organizationId}`;
    field("role").textContent = data.roleTitle;

    $$('[data-status-form] input[name="status"]', drawer).forEach((radio) => {
      radio.checked = radio.value === data.status;
    });

    setLink(field("careers-link"), data.careersUrl);
    setLink(field("posting-link"), data.postingUrl);
    field("careers-missing").hidden = Boolean(data.careersUrl);
    field("careers-missing-link").href = `/organizations?organization=${data.organizationId}`;
    $("[data-checked-form]", drawer).hidden = data.portalCheck !== "true";

    field("salary").textContent = data.salary;
    field("location").textContent = data.location;
    field("applied-label").textContent = data.appliedAt ? "Applied" : "Added";
    field("applied").textContent = data.appliedAt ? data.appliedAtDisplay : data.addedDisplay;
    field("applied-ago").textContent = data.appliedAgo;
    renderHistory(data.history);
    field("last-checked").textContent = data.lastChecked;
    field("last-checked-detail").textContent = data.lastCheckedDetail;
    field("due").hidden = data.due !== "true";

    const notes = field("notes");
    notes.textContent = data.notes || "No notes yet. Use Edit details to add recruiter names, next steps, or anything else worth remembering.";
    notes.classList.toggle("muted", !data.notes);

    setDrawerActions(data.applicationId);
  };

  const fillEditForm = (data) => {
    const set = (name, value) => {
      const element = editForm.elements.namedItem(name);
      if (element) element.value = value;
    };
    set("organization_name", data.organizationName);
    set("role_title", data.roleTitle);
    set("status", data.status);
    set("work_location", data.workLocation);
    set("posting_url", data.postingUrl);
    set("salary_min", data.salaryMin);
    set("salary_max", data.salaryMax);
    set("applied_at", data.appliedAt);
    set("notes", data.notes);
  };

  const showDrawerView = () => {
    $$("[data-drawer-view]", drawer).forEach((section) => {
      section.hidden = false;
    });
    editForm.hidden = true;
  };

  const showDrawerEdit = () => {
    $$("[data-drawer-view]", drawer).forEach((section) => {
      section.hidden = true;
    });
    editForm.hidden = false;
    (editForm.querySelector('[aria-invalid="true"]') || editForm.elements.namedItem("role_title")).focus();
  };

  const findApplication = (id) => $(`[data-open-application][data-application-id="${id}"]`);

  const openApplication = (data) => {
    if (!drawer) return;
    clearFormErrors(editForm);
    fillDrawer(data);
    fillEditForm(data);
    showDrawerView();
    openDialog(drawer);
  };

  drawer?.addEventListener("change", (event) => {
    if (event.target.matches('[data-status-form] input[name="status"]')) {
      event.target.form.requestSubmit();
    }
  });

  drawer?.addEventListener("swapped", (event) => {
    const form = event.target;
    if (form.matches("[data-delete-form]")) {
      drawer.close();
      return;
    }
    const row = findApplication(currentApplicationID);
    if (row) {
      fillDrawer(row.dataset);
      fillEditForm(row.dataset);
      return;
    }

    // The row can leave the page (for example, the due list), so reflect the change directly.
    const data = { ...currentApplication };
    if (form.matches("[data-checked-form]")) {
      Object.assign(data, { lastChecked: "Today", lastCheckedDetail: "", due: "false" });
    } else if (form.matches("[data-status-form]")) {
      const status = form.elements.namedItem("status").value;
      if (status !== data.status) {
        const portalCheck = status === "applied" || status === "in_contact";
        const option = $(`input[name="status"][value="${status}"]`, form)?.closest(".status-option");
        const statusLabel = option?.textContent.trim() || data.statusLabel;
        const statusClass = Array.from(option?.classList || []).find((name) => name.startsWith("status-")) || "";
        const history = parseHistory(data.history);
        history.push({ label: statusLabel, statusClass, date: "Today", ago: "" });
        Object.assign(data, {
          status,
          statusLabel,
          statusSince: "Today",
          history: JSON.stringify(history),
          portalCheck: String(portalCheck),
          due: portalCheck ? data.due : "false",
        });
      }
    }
    fillDrawer(data);
    fillEditForm(data);
  });

  drawer?.addEventListener("close", () => {
    const confirm = $("[data-confirm]", drawer);
    if (confirm) resetConfirm(confirm);
  });

  // Organization dialog

  const organizationDialog = $("#organization-dialog");
  const organizationForm = $("[data-organization-form]");

  const fillOrganization = (data, fillFields = true) => {
    const mark = $('[data-org-field="mark"]', organizationDialog);
    mark.className = `org-mark org-mark-large ${data.organizationMark}`;
    mark.textContent = data.organizationInitial;
    $('[data-org-field="title"]', organizationDialog).textContent = data.organizationName;
    const count = Number(data.applicationCount);
    const open = Number(data.openApplicationCount);
    $('[data-org-field="summary"]', organizationDialog).textContent = count === 0
      ? "No applications yet"
      : `${count} ${count === 1 ? "application" : "applications"}${open ? ` · ${open} open` : ""}`;
    $('[data-org-field="applications-link"]', organizationDialog).href = `/applications?organization=${data.organizationId}`;
    organizationForm.action = `/organizations/${data.organizationId}`;
    if (fillFields) {
      organizationForm.elements.namedItem("name").value = data.organizationName;
      organizationForm.elements.namedItem("careers_url").value = data.careersUrl;
    }
  };

  const openOrganization = (data) => {
    clearFormErrors(organizationForm);
    fillOrganization(data);
    openDialog(organizationDialog);
    const careersURL = organizationForm.elements.namedItem("careers_url");
    (careersURL.value ? organizationForm.elements.namedItem("name") : careersURL).focus();
  };

  // Settings

  const settingsDialog = $("#settings-dialog");
  const settingsForm = $("[data-settings-form]");
  let savingSettings = false;

  const openSettings = () => {
    settingsForm.reset();
    applySalaryPrivacy(document.documentElement.dataset.salaryPrivacy === "true");
    const theme = readThemePreference();
    $$("[data-theme-choice]", settingsForm).forEach((choice) => {
      choice.checked = choice.value === theme;
    });
    $("[data-settings-error]", settingsForm).hidden = true;
    openDialog(settingsDialog);
  };

  settingsDialog?.addEventListener("cancel", (event) => {
    if (savingSettings) event.preventDefault();
  });

  settingsForm?.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (savingSettings) return;
    savingSettings = true;
    const submit = settingsForm.querySelector('[type="submit"]');
    const error = $("[data-settings-error]", settingsForm);
    submit.disabled = true;
    submit.textContent = "Saving…";
    error.hidden = true;
    try {
      const body = new URLSearchParams(new FormData(settingsForm));
      body.delete("theme");
      const response = await fetch(settingsForm.action, { method: "POST", body });
      if (!response.ok) throw new Error(await response.text());
      setFlash("Settings saved");
      window.location.reload();
    } catch (failure) {
      error.textContent = failure.message || "Unable to save settings. Please try again.";
      error.hidden = false;
    } finally {
      savingSettings = false;
      submit.disabled = false;
      submit.textContent = "Save settings";
    }
  });

  settingsDialog?.addEventListener("change", (event) => {
    if (event.target.matches("input[data-salary-privacy]")) {
      setSalaryPrivacy(event.target.checked);
    } else if (event.target.matches("[data-theme-choice]")) {
      setTheme(event.target.value);
    }
  });

  // Search

  const searchInput = () => $("[data-search-input]");

  // Chips narrow the list to rows tagged with the chosen value, alongside the text search.
  let activeChip = "";

  const applySearch = () => {
    const input = searchInput();
    if (!input) return;
    const terms = input.value.toLowerCase().split(/\s+/).filter(Boolean);
    const items = $$("[data-search-item]");
    let shown = 0;
    items.forEach((item) => {
      const text = item.dataset.searchText.toLowerCase();
      const tagged = !activeChip || (item.dataset.tags || "").split(/\s+/).includes(activeChip);
      const match = tagged && terms.every((term) => text.includes(term));
      item.hidden = !match;
      if (match) shown += 1;
    });
    const empty = $("[data-search-empty]");
    if (empty) {
      empty.hidden = shown > 0 || items.length === 0;
      const term = $("[data-search-term]", empty);
      if (term) term.textContent = input.value.trim();
    }
    const count = $("[data-search-count]");
    if (count) count.textContent = shown;
  };

  let searchNavigation;
  const onSearchInput = (input) => {
    applySearch();
    const form = input.closest("[data-filter-form]");
    if (!form) return;

    const url = new URL(window.location.href);
    if (input.value.trim()) {
      url.searchParams.set("q", input.value.trim());
    } else {
      url.searchParams.delete("q");
    }
    history.replaceState(null, "", url);
    const returnPath = url.pathname + url.search;
    $$('input[name="return_to"]').forEach((hidden) => {
      hidden.value = returnPath;
    });

    // The server already narrowed the list; widening the search needs a fresh page.
    const serverQuery = (input.dataset.serverQuery || "").toLowerCase();
    clearTimeout(searchNavigation);
    const narrowsServerQuery = serverQuery.split(/\s+/).every((term) => input.value.toLowerCase().includes(term));
    if (serverQuery && !narrowsServerQuery) {
      searchNavigation = setTimeout(() => form.requestSubmit(), 350);
    }
  };

  // Forms that update the page in place

  const swapFrom = (html) => {
    const next = new DOMParser().parseFromString(html, "text/html");
    $$("[data-swap]", next).forEach((fresh) => {
      const current = $(`[data-swap="${fresh.dataset.swap}"]`);
      if (current) current.replaceWith(document.adoptNode(fresh));
    });
    applySearch();
  };

  const confirmTimers = new WeakMap();
  const resetConfirm = (button) => {
    clearTimeout(confirmTimers.get(button));
    button.classList.remove("is-confirming");
    const label = $("span", button);
    if (label && button.dataset.label) label.textContent = button.dataset.label;
  };
  const armConfirm = (button) => {
    const label = $("span", button);
    button.dataset.label = label.textContent;
    label.textContent = button.dataset.confirm;
    button.classList.add("is-confirming");
    confirmTimers.set(button, setTimeout(() => resetConfirm(button), 4000));
  };

  const submitInPlace = async (form) => {
    if (form.getAttribute("aria-busy") === "true") return;
    const confirm = $("[data-confirm]", form);
    if (confirm && !confirm.classList.contains("is-confirming")) {
      armConfirm(confirm);
      return;
    }
    if (confirm) resetConfirm(confirm);

    form.setAttribute("aria-busy", "true");
    form.closest(".list-row")?.classList.add("is-busy");
    try {
      const response = await fetch(form.action, {
        method: "POST",
        body: new URLSearchParams(new FormData(form)),
      });
      if (!response.ok) throw new Error(`Request failed with ${response.status}`);
      if (new URL(response.url).pathname !== window.location.pathname) {
        setFlash(form.dataset.toast);
        window.location.assign(response.url);
        return;
      }
      swapFrom(await response.text());
      form.dispatchEvent(new CustomEvent("swapped", { bubbles: true }));
      toast(form.dataset.toast);
    } catch {
      form.closest(".list-row")?.classList.remove("is-busy");
      toast("That didn't save. Please try again.", "error");
    } finally {
      form.removeAttribute("aria-busy");
    }
  };

  document.addEventListener("submit", (event) => {
    const form = event.target;
    if (form.matches("[data-swap-form]")) {
      event.preventDefault();
      submitInPlace(form);
      return;
    }
    if (form.matches("[data-flash]")) {
      setFlash(form.dataset.flash);
    }
  });

  // Board drag and drop. Every card also opens the details panel, where the status can be changed without dragging.

  let draggedCard = null;

  const clearDropTargets = () => {
    $$("[data-drop-status].is-over").forEach((zone) => zone.classList.remove("is-over"));
  };

  document.addEventListener("dragstart", (event) => {
    const card = event.target.closest?.("[data-board-card]");
    if (!card) return;
    draggedCard = card;
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", $("[data-open-application]", card).dataset.applicationId);
    card.classList.add("is-dragging");
    document.body.classList.add("is-dragging-card");
  });

  document.addEventListener("dragend", () => {
    draggedCard?.classList.remove("is-dragging");
    draggedCard = null;
    document.body.classList.remove("is-dragging-card");
    clearDropTargets();
  });

  // Closed columns hold the specific drop zones, so the innermost target wins.
  const dropTarget = (event) => (draggedCard ? event.target.closest?.("[data-drop-status]") : null);

  document.addEventListener("dragover", (event) => {
    const zone = dropTarget(event);
    if (!zone) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    if (!zone.classList.contains("is-over")) {
      clearDropTargets();
      zone.classList.add("is-over");
    }
  });

  document.addEventListener("dragleave", (event) => {
    const zone = dropTarget(event);
    if (zone && !zone.contains(event.relatedTarget)) zone.classList.remove("is-over");
  });

  document.addEventListener("drop", (event) => {
    const zone = dropTarget(event);
    if (!zone) return;
    event.preventDefault();
    const card = draggedCard;
    const data = $("[data-open-application]", card).dataset;
    const status = zone.dataset.dropStatus;
    if (status === data.status) return;
    const form = $("[data-board-form]");
    form.action = `/applications/${data.applicationId}/status`;
    form.elements.namedItem("status").value = status;
    form.dataset.toast = `${data.organizationName} moved to ${zone.querySelector("h2")?.textContent || zone.textContent.trim()}`;
    card.classList.add("is-busy");
    submitInPlace(form).finally(() => card.classList.remove("is-busy"));
  });

  // Global event delegation

  document.addEventListener("click", (event) => {
    const target = event.target;

    if (target instanceof HTMLDialogElement) {
      if (pointerStartedOnBackdrop && !(target === settingsDialog && savingSettings)) target.close();
      return;
    }
    if (!(target instanceof Element)) return;

    if (target.closest("[data-close]")) {
      const dialog = target.closest("dialog");
      if (!(dialog === settingsDialog && savingSettings)) dialog?.close();
      return;
    }
    if (target.closest("[data-open-create]")) {
      openCreate();
      return;
    }
    if (target.closest("[data-open-settings]")) {
      openSettings();
      return;
    }
    if (target.closest("[data-toggle-salaries]")) {
      setSalaryPrivacy(document.documentElement.dataset.salaryPrivacy !== "true");
      return;
    }
    const application = target.closest("[data-open-application]");
    if (application) {
      openApplication(application.dataset);
      return;
    }
    const organization = target.closest("[data-open-organization]");
    if (organization) {
      openOrganization(organization.dataset);
      return;
    }
    if (target.closest("[data-edit]")) {
      showDrawerEdit();
      return;
    }
    if (target.closest("[data-cancel-edit]")) {
      clearFormErrors(editForm);
      const row = findApplication(currentApplicationID);
      if (row) fillEditForm(row.dataset);
      showDrawerView();
      return;
    }
    const chip = target.closest("[data-chip]");
    if (chip) {
      activeChip = chip.dataset.chip;
      $$("[data-chip]").forEach((other) => other.setAttribute("aria-pressed", String(other === chip)));
      applySearch();
      return;
    }
    const portal = target.closest("[data-portal-link]");
    if (portal) {
      // Nudge toward the next step once the portal has been opened.
      portal.closest(".list-row")?.classList.add("is-ready");
      if (drawer?.contains(portal)) $("[data-checked-form] button", drawer)?.classList.add("is-ready");
    }
  });

  document.addEventListener("input", (event) => {
    if (event.target.matches("[data-search-input]")) onSearchInput(event.target);
  });

  document.addEventListener("change", (event) => {
    if (event.target.matches("[data-autosubmit]")) event.target.form.requestSubmit();
  });

  document.addEventListener("keydown", (event) => {
    const target = event.target;

    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      const form = target.closest?.("dialog form:not([data-swap-form])");
      if (form) {
        event.preventDefault();
        form.requestSubmit();
      }
      return;
    }

    if (target.matches?.("[data-search-input]") && event.key === "Escape" && target.value) {
      event.preventDefault();
      target.value = "";
      onSearchInput(target);
      return;
    }

    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
    if (target.isContentEditable || target.matches?.("input, textarea, select")) return;
    if ($("dialog[open]")) return;

    if (event.key === "n" || event.key === "N") {
      event.preventDefault();
      openCreate();
    } else if (event.key === "/") {
      const input = searchInput();
      if (input) {
        event.preventDefault();
        input.focus();
        input.select();
      }
    }
  });

  // Initial state

  applySalaryPrivacy(document.documentElement.dataset.salaryPrivacy === "true");
  applySearch();

  // Validation errors render at the POST URL, which isn't a page that can be reloaded.
  const restoreURL = (form) => {
    const path = form?.elements.namedItem("return_to")?.value;
    if (path) history.replaceState(null, "", path);
  };

  if (createDialog?.hasAttribute("data-open-on-load")) {
    restoreURL(createForm);
    openCreate();
    focusFirstError(createForm);
  }

  if (drawer?.hasAttribute("data-open-on-load")) {
    restoreURL(editForm);
    const id = drawer.dataset.applicationId;
    const row = findApplication(id);
    if (row) {
      fillDrawer(row.dataset);
    } else {
      currentApplicationID = id;
      setDrawerActions(id);
    }
    openDialog(drawer);
    showDrawerEdit();
  }

  let flash = takeFlash();
  if (organizationDialog?.hasAttribute("data-open-on-load")) {
    const id = organizationDialog.dataset.organizationId;
    const row = $(`[data-open-organization][data-organization-id="${id}"]`);
    if (organizationDialog.hasAttribute("data-start-saved")) {
      flash = flash || "Organization saved";
      history.replaceState(null, "", "/organizations");
      const savedRow = row?.closest(".list-row");
      savedRow?.classList.add("is-highlighted");
      savedRow?.scrollIntoView({ block: "center" });
    } else {
      if (row) fillOrganization(row.dataset, !organizationDialog.hasAttribute("data-start-with-errors"));
      history.replaceState(null, "", "/organizations");
      openDialog(organizationDialog);
      focusFirstError(organizationDialog);
    }
  }

  if (flash && !$(".form-alert[role='alert']:not([hidden])")) toast(flash);
})();
