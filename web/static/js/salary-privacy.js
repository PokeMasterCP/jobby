(() => {
  try {
    if (localStorage.getItem("jobby.hideSalaries") === "true") {
      document.documentElement.dataset.salaryPrivacy = "true";
    }
  } catch {
    // Storage may be unavailable in a restricted browser session.
  }
})();
