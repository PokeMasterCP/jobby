(() => {
  const dialog = document.querySelector("#application-dialog");
  const openButton = document.querySelector("[data-open-application-dialog]");

  if (!dialog || !openButton) {
    return;
  }

  openButton.addEventListener("click", () => {
    dialog.showModal();
  });

  if (dialog.hasAttribute("data-open-on-load")) {
    dialog.showModal();
    dialog.querySelector('[aria-invalid="true"]')?.focus();
  }

  dialog.querySelectorAll("[data-close-application-dialog]").forEach((button) => {
    button.addEventListener("click", () => {
      dialog.close();
    });
  });

  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) {
      dialog.close();
    }
  });

})();
