async function refreshStatus() {
  const status = document.getElementById("status");
  try {
    const response = await fetch("/api/health", { headers: { Accept: "application/json" } });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    status.textContent = "Server ready";
  } catch (error) {
    status.textContent = "Server unavailable";
  }
}

refreshStatus();
