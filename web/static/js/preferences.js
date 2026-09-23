// Runs before the stylesheet so saved display preferences apply without a flash.
(() => {
  const root = document.documentElement;
  const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
  root.classList.add("js");

  const read = (key) => {
    try {
      return localStorage.getItem(key);
    } catch {
      // Storage may be unavailable in a restricted browser session.
      return null;
    }
  };

  const applyTheme = () => {
    const preference = read("jobby.theme");
    const dark = preference === "dark" || (preference !== "light" && systemDark.matches);
    root.dataset.theme = dark ? "dark" : "light";
  };

  applyTheme();
  systemDark.addEventListener("change", applyTheme);
  window.addEventListener("storage", (event) => {
    if (event.key === "jobby.theme" || event.key === null) applyTheme();
  });

  if (read("jobby.hideSalaries") === "true") {
    root.dataset.salaryPrivacy = "true";
  }
})();
