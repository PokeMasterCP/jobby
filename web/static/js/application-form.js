(() => {
  const bindCareerPortalPlaceholder = (form) => {
    const organizationField = form?.elements.namedItem("organization_name");
    const careersURLField = form?.elements.namedItem("careers_url");
    if (!organizationField || !careersURLField) {
      return () => {};
    }

    const updatePlaceholder = () => {
      const organizationName = organizationField.value.trim() || "Organization";
      careersURLField.placeholder = `${organizationName}'s Career Portal link`;
    };

    organizationField.addEventListener("input", updatePlaceholder);
    updatePlaceholder();
    return updatePlaceholder;
  };

  const createDialog = document.querySelector("#application-dialog");
  const createOpenButton = document.querySelector("[data-open-application-dialog]");

  if (createDialog && createOpenButton) {
    bindCareerPortalPlaceholder(createDialog.querySelector("form"));

    createOpenButton.addEventListener("click", () => {
      createDialog.showModal();
    });

    if (createDialog.hasAttribute("data-open-on-load")) {
      createDialog.showModal();
      createDialog.querySelector('[aria-invalid="true"]')?.focus();
    }

    createDialog.querySelectorAll("[data-close-application-dialog]").forEach((button) => {
      button.addEventListener("click", () => {
        createDialog.close();
      });
    });

    createDialog.addEventListener("click", (event) => {
      if (event.target === createDialog) {
        createDialog.close();
      }
    });
  }

  const organizationDetailDialog = document.querySelector("#organization-detail-dialog");
  if (organizationDetailDialog) {
    const organizationForm = organizationDetailDialog.querySelector("[data-organization-edit-form]");
    const organizationName = organizationDetailDialog.querySelector("[data-organization-detail-name]");
    const savedMessage = organizationDetailDialog.querySelector("[data-organization-detail-success]");

    const populateOrganization = (organization, showSaved = false) => {
      organizationName.textContent = organization.organizationName;
      organizationForm.action = `/organizations/${organization.organizationId}`;
      organizationForm.elements.namedItem("name").value = organization.organizationName;
      organizationForm.elements.namedItem("careers_url").value = organization.careersUrl;
      organizationDetailDialog.querySelector("[data-organization-application-count]").textContent = organization.applicationCount;
      organizationDetailDialog.querySelector("[data-organization-open-count]").textContent = organization.openApplicationCount;
      savedMessage.hidden = !showSaved;
    };

    const openOrganization = (row, showSaved = false) => {
      populateOrganization(row.dataset, showSaved);
      organizationDetailDialog.showModal();
    };

    const organizationRows = Array.from(document.querySelectorAll("[data-open-organization-detail]"));
    organizationRows.forEach((row) => {
      row.addEventListener("click", () => {
        openOrganization(row);
      });
    });

    organizationDetailDialog.querySelectorAll("[data-close-organization-detail]").forEach((button) => {
      button.addEventListener("click", () => {
        organizationDetailDialog.close();
      });
    });

    organizationDetailDialog.addEventListener("click", (event) => {
      if (event.target === organizationDetailDialog) {
        organizationDetailDialog.close();
      }
    });

    if (organizationDetailDialog.hasAttribute("data-open-on-load")) {
      const organizationID = organizationDetailDialog.dataset.openOrganizationId;
      const row = organizationRows.find((candidate) => candidate.dataset.organizationId === organizationID);
      if (row && !organizationDetailDialog.hasAttribute("data-start-with-errors")) {
        populateOrganization(row.dataset, organizationDetailDialog.hasAttribute("data-start-saved"));
      } else {
        organizationForm.action = `/organizations/${organizationID}`;
        organizationName.textContent = organizationForm.elements.namedItem("name").value;
        if (row) {
          organizationDetailDialog.querySelector("[data-organization-application-count]").textContent = row.dataset.applicationCount;
          organizationDetailDialog.querySelector("[data-organization-open-count]").textContent = row.dataset.openApplicationCount;
        }
      }
      organizationDetailDialog.showModal();
      organizationDetailDialog.querySelector('[aria-invalid="true"]')?.focus();
    }
  }

  const detailDialog = document.querySelector("#application-detail-dialog");
  if (!detailDialog) {
    return;
  }

  const summary = detailDialog.querySelector("[data-application-summary]");
  const editForm = detailDialog.querySelector("[data-application-edit-form]");
  const statusForm = detailDialog.querySelector("[data-change-application-status]");
  const checkedForm = detailDialog.querySelector("[data-mark-application-checked]");
  const deleteForm = detailDialog.querySelector("[data-delete-application]");
  const kicker = detailDialog.querySelector("[data-detail-kicker]");
  const organization = detailDialog.querySelector("[data-detail-organization]");
  const statusSelect = detailDialog.querySelector("[data-detail-status]");
  const careersLink = detailDialog.querySelector("[data-detail-careers-url]");
  const careersMissing = detailDialog.querySelector("[data-detail-careers-missing]");
  const postingLink = detailDialog.querySelector("[data-detail-posting-url]");
  const postingMissing = detailDialog.querySelector("[data-detail-posting-missing]");

  const setText = (selector, value) => {
    const element = detailDialog.querySelector(selector);
    if (element) {
      element.textContent = value;
    }
  };

  const setField = (name, value) => {
    const field = editForm?.elements.namedItem(name);
    if (field) {
      field.value = value;
    }
  };

  const setActions = (applicationID) => {
    if (editForm) {
      editForm.action = `/applications/${applicationID}`;
    }
    if (statusForm) {
      statusForm.action = `/applications/${applicationID}/status`;
    }
    if (checkedForm) {
      checkedForm.action = `/applications/${applicationID}/checked`;
    }
    if (deleteForm) {
      deleteForm.action = `/applications/${applicationID}/delete`;
    }
  };

  const setResourceLink = (link, missingMessage, url) => {
    if (url) {
      link.href = url;
      link.hidden = false;
      missingMessage.hidden = true;
      return;
    }

    link.removeAttribute("href");
    link.hidden = true;
    missingMessage.hidden = false;
  };

  const populateSummary = (application) => {
    organization.textContent = application.organizationName;
    setText("[data-detail-role]", application.roleTitle);
    setText("[data-detail-salary]", application.salary);
    setText("[data-detail-location]", application.location);
    setText("[data-detail-applied-at]", application.appliedAtDisplay);
    setText("[data-detail-last-checked]", application.lastChecked);
    setText("[data-detail-notes]", application.notes || "No notes yet.");

    statusSelect.value = application.status;
    statusSelect.className = `status-tag status-quick-select ${application.statusClass}`;
    statusSelect.setAttribute("aria-label", `Change status, currently ${application.statusLabel}`);

    setResourceLink(careersLink, careersMissing, application.careersUrl);
    careersMissing.href = `/organizations?organization=${application.organizationId}`;
    setResourceLink(postingLink, postingMissing, application.postingUrl);

    setActions(application.applicationId);
  };

  const populateEditForm = (application) => {
    setField("organization_name", application.organizationName);
    setField("role_title", application.roleTitle);
    setField("status", application.status);
    setField("work_location", application.workLocation);
    setField("posting_url", application.postingUrl);
    setField("salary_min", application.salaryMin);
    setField("salary_max", application.salaryMax);
    setField("applied_at", application.appliedAt);
    setField("notes", application.notes);
  };

  const showSummary = () => {
    kicker.textContent = "05 / Application brief";
    summary.hidden = false;
    editForm.hidden = true;
  };

  const showEditForm = () => {
    kicker.textContent = "05 / Edit record";
    summary.hidden = true;
    editForm.hidden = false;
    editForm.querySelector('[aria-invalid="true"]')?.focus() ||
      editForm.elements.namedItem("organization_name")?.focus();
  };

  const openApplication = (row) => {
    populateSummary(row.dataset);
    populateEditForm(row.dataset);
    showSummary();
    detailDialog.showModal();
  };

  const applicationRows = Array.from(document.querySelectorAll("[data-open-application-summary]"));
  applicationRows.forEach((row) => {
    row.addEventListener("click", () => {
      openApplication(row);
    });
  });

  document.querySelectorAll("[data-open-application-reference]").forEach((button) => {
    button.addEventListener("click", () => {
      const applicationID = button.dataset.openApplicationReference;
      const row = applicationRows.find((candidate) => candidate.dataset.applicationId === applicationID);
      if (row) {
        openApplication(row);
      }
    });
  });

  detailDialog.querySelector("[data-edit-application]")?.addEventListener("click", showEditForm);
  detailDialog.querySelector("[data-cancel-application-edit]")?.addEventListener("click", showSummary);
  statusSelect?.addEventListener("change", () => {
    statusForm?.requestSubmit();
  });
  deleteForm?.addEventListener("submit", (event) => {
    if (!window.confirm(`Delete the ${organization.textContent} application? This cannot be undone.`)) {
      event.preventDefault();
    }
  });
  detailDialog.querySelector("[data-close-application-detail]")?.addEventListener("click", () => {
    detailDialog.close();
  });

  detailDialog.addEventListener("click", (event) => {
    if (event.target === detailDialog) {
      detailDialog.close();
    }
  });

  if (detailDialog.hasAttribute("data-open-on-load")) {
    const applicationID = detailDialog.dataset.openApplicationId;
    const row = applicationRows.find((candidate) => candidate.dataset.applicationId === applicationID);
    if (row) {
      populateSummary(row.dataset);
    } else {
      setActions(applicationID);
    }
    showEditForm();
    detailDialog.showModal();
  }
})();
