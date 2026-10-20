(function () {
	const HISTORY_KEY = "goshort.history";
	const HISTORY_LIMIT = 10;

	const form = document.getElementById("shorten-form");

	document.querySelectorAll("[data-copy]").forEach((btn) => {
		btn.addEventListener("click", () => copy(btn.dataset.copy, btn));
	});

	if (!form) {
		return;
	}

	const urlInput = document.getElementById("url");
	const aliasInput = document.getElementById("alias");
	const ttlSelect = document.getElementById("ttl");
	const submitBtn = document.getElementById("submit");
	const errorBox = document.getElementById("error");
	const result = document.getElementById("result");
	const shortLink = document.getElementById("short-url");
	const copyBtn = document.getElementById("copy");

	form.addEventListener("submit", async (e) => {
		e.preventDefault();
		hideError();
		result.hidden = true;

		const url = urlInput.value.trim();
		if (!url) {
			showError("Введите ссылку, которую нужно сократить");
			urlInput.focus();
			return;
		}

		submitBtn.disabled = true;
		submitBtn.textContent = "Сокращаем...";
		try {
			const resp = await fetch("/api/shorten", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					url: url,
					alias: aliasInput.value.trim(),
					ttl_hours: Number(ttlSelect.value),
				}),
			});
			const data = await resp.json();
			if (!resp.ok) {
				showError(data.error || "Не удалось сократить ссылку");
				return;
			}
			showResult(data);
			saveHistory(data);
			renderHistory();
			urlInput.value = "";
			aliasInput.value = "";
		} catch (err) {
			showError("Сервер недоступен, попробуйте позже");
		} finally {
			submitBtn.disabled = false;
			submitBtn.textContent = "Сократить";
		}
	});

	copyBtn.addEventListener("click", () => copy(shortLink.href, copyBtn));

	document.getElementById("clear-history").addEventListener("click", () => {
		try {
			localStorage.removeItem(HISTORY_KEY);
		} catch (e) {}
		renderHistory();
	});

	function showResult(data) {
		shortLink.href = data.short_url;
		shortLink.textContent = data.short_url;
		document.getElementById("original").textContent = "Исходная ссылка: " + data.original_url;
		document.getElementById("expires").textContent = data.expires_at
			? "Действует до " + formatDate(data.expires_at)
			: "Срок действия не ограничен";
		document.getElementById("stats-link").href = "/stats/" + encodeURIComponent(data.code);
		result.hidden = false;
	}

	function showError(text) {
		errorBox.textContent = text.charAt(0).toUpperCase() + text.slice(1);
		errorBox.hidden = false;
	}

	function hideError() {
		errorBox.hidden = true;
	}

	function loadHistory() {
		try {
			return JSON.parse(localStorage.getItem(HISTORY_KEY)) || [];
		} catch (e) {
			return [];
		}
	}

	function saveHistory(data) {
		const items = loadHistory().filter((it) => it.code !== data.code);
		items.unshift({ code: data.code, short_url: data.short_url, original_url: data.original_url });
		try {
			localStorage.setItem(HISTORY_KEY, JSON.stringify(items.slice(0, HISTORY_LIMIT)));
		} catch (e) {}
	}

	function renderHistory() {
		const list = document.getElementById("history-list");
		const items = loadHistory();
		list.innerHTML = "";
		document.getElementById("history-empty").hidden = items.length > 0;

		for (const it of items) {
			const li = document.createElement("li");

			const a = document.createElement("a");
			a.className = "h-short";
			a.href = it.short_url;
			a.target = "_blank";
			a.textContent = it.short_url.replace(/^https?:\/\//, "");

			const orig = document.createElement("div");
			orig.className = "h-orig";
			orig.textContent = it.original_url;

			const stats = document.createElement("a");
			stats.className = "h-stats";
			stats.href = "/stats/" + encodeURIComponent(it.code);
			stats.textContent = "Статистика";

			li.append(a, stats, orig);
			list.append(li);
		}
	}

	renderHistory();

	function formatDate(iso) {
		return new Date(iso).toLocaleString("ru-RU", {
			day: "2-digit",
			month: "2-digit",
			year: "numeric",
			hour: "2-digit",
			minute: "2-digit",
		});
	}

	async function copy(text, btn) {
		try {
			await navigator.clipboard.writeText(text);
			const old = btn.textContent;
			btn.textContent = "Скопировано";
			setTimeout(() => (btn.textContent = old), 1500);
		} catch (e) {
			window.prompt("Скопируйте ссылку", text);
		}
	}
})();
