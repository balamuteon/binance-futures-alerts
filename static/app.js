// Функция для тестовой кнопки
function playSoundTest() {
    console.log("НАЖАТА КНОПКА 'Послушать тестовый сигнал'");
    const testPlayer = document.getElementById('testAudioPlayer');
    if (testPlayer) {
        testPlayer.play()
            .then(() => {
                console.log("Тестовый звук (с кнопки) УСПЕШНО воспроизведен!");
                alert("Если все хорошо, вы услышали короткий сигнал.");
            })
            .catch(error => {
                console.error("Ошибка тестового воспроизведения звука (с кнопки):", error);
                alert("Не удалось воспроизвести звук: " + error.name + " - " + error.message);
            });
    } else {
        console.error("Тестовый плеер #testAudioPlayer не найден!");
    }
}

// Основной скрипт приложения
document.addEventListener("DOMContentLoaded", function() {
    console.log("ОСНОВНОЙ СКРИПТ: DOMContentLoaded сработал.");

    const testSoundButton = document.getElementById("testSoundButton");
    if (testSoundButton) {
        testSoundButton.addEventListener("click", playSoundTest);
    }

    const alertsList = document.getElementById("alertsList");
    const statusDiv = document.getElementById("status");
    const appShell = document.querySelector(".app-shell");
    const chartPanel = document.getElementById("chartPanel");
    const chartPanelFrame = chartPanel ? chartPanel.querySelector(".chart-panel__frame") : null;
    const chartPanelSymbol = document.getElementById("chartPanelSymbol");
    const chartPanelOpenLink = document.getElementById("chartPanelOpen");
    const chartPanelCloseButton = document.getElementById("chartPanelClose");

    let chartActiveLink = null;
    let chartActiveSymbol = null;
    let socket;

    const alertSound = new Audio('alert.wav');
    alertSound.preload = 'auto';

    function ensureChartIframe(symbol) {
        if (!chartPanelFrame) { return; }

        let iframe = chartPanelFrame.querySelector("iframe");
        const embedUrl = `https://www.tradingview.com/widgetembed/?symbol=${encodeURIComponent(symbol)}&interval=15&symboledit=1&saveimage=0&toolbarbg=f1f3f6&studies=[]&hideideas=1&theme=dark&style=1&timezone=Etc/UTC&hidevolume=1&withdateranges=0&allow_symbol_change=1&locale=ru`;

        if (!iframe) {
            iframe = document.createElement("iframe");
            iframe.setAttribute("allowtransparency", "true");
            iframe.setAttribute("scrolling", "no");
            iframe.loading = "lazy";
            iframe.title = "TradingView preview";
            iframe.referrerPolicy = "no-referrer-when-downgrade";
            chartPanelFrame.appendChild(iframe);
        }

        if (iframe.dataset.loadedSymbol !== symbol) {
            iframe.src = embedUrl;
            iframe.dataset.loadedSymbol = symbol;
        }
    }

    function setActiveSymbolLink(link) {
        if (!alertsList) { return; }
        const activeLinks = alertsList.querySelectorAll(".alert-symbol--active");
        activeLinks.forEach(activeLink => {
            if (activeLink !== link) {
                activeLink.classList.remove("alert-symbol--active");
            }
        });

        if (link) {
            link.classList.add("alert-symbol--active");
        }
    }

    function openChartPanel(link) {
        if (!chartPanel || !chartPanelOpenLink || !appShell) { return; }
        if (!link || !link.dataset.tvSymbol) { return; }

        const symbol = link.dataset.tvSymbol;
        ensureChartIframe(symbol);

        if (chartPanelSymbol) {
            const baseSymbol = link.dataset.rawSymbol || (link.textContent ? link.textContent.trim() : symbol);
            chartPanelSymbol.textContent = baseSymbol || symbol;
            chartPanelSymbol.title = symbol;
        }

        chartPanel.dataset.symbol = symbol;
        chartPanelOpenLink.href = link.href;
        chartPanelOpenLink.removeAttribute("tabindex");
        if (chartPanelCloseButton) {
            chartPanelCloseButton.removeAttribute("tabindex");
        }

        chartActiveSymbol = symbol;
        chartActiveLink = link;
        setActiveSymbolLink(link);

        appShell.classList.add("app-shell--chart-open");
        chartPanel.setAttribute("aria-hidden", "false");
    }

    function closeChartPanel() {
        if (!chartPanel || !appShell) { return; }
        chartPanel.removeAttribute("data-symbol");
        chartPanel.setAttribute("aria-hidden", "true");
        appShell.classList.remove("app-shell--chart-open");
        if (chartPanelSymbol) {
            chartPanelSymbol.textContent = "—";
            chartPanelSymbol.removeAttribute("title");
        }
        if (chartPanelOpenLink) {
            chartPanelOpenLink.href = "#";
            chartPanelOpenLink.setAttribute("tabindex", "-1");
        }
        if (chartPanelCloseButton) {
            chartPanelCloseButton.setAttribute("tabindex", "-1");
        }
        chartActiveSymbol = null;
        chartActiveLink = null;
        setActiveSymbolLink(null);
    }

    function isModifiedEvent(event) {
        return event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
    }

    function handleSymbolClick(event) {
        const link = event.currentTarget;
        event.stopPropagation();
        if (isModifiedEvent(event)) { return; }
        event.preventDefault();

        if (chartActiveSymbol === link.dataset.tvSymbol) {
            openChartPanel(link);
            return;
        }

        openChartPanel(link);
    }

    function handleAlertCardClick(event) {
        if (isModifiedEvent(event)) { return; }

        const link = event.currentTarget.querySelector(".alert-symbol");
        if (!link) { return; }

        event.preventDefault();
        openChartPanel(link);
    }

    function handleAlertCardKeydown(event) {
        if (event.key !== "Enter" && event.key !== " ") { return; }

        const link = event.currentTarget.querySelector(".alert-symbol");
        if (!link) { return; }

        event.preventDefault();
        openChartPanel(link);
    }

    function handleDocumentKeydown(event) {
        if (event.key === "Escape" && chartPanel && chartPanel.getAttribute("aria-hidden") === "false") {
            closeChartPanel();
        }
    }

    function syncChartPanelLink() {
        if (!alertsList || !chartActiveSymbol) {
            chartActiveLink = null;
            setActiveSymbolLink(null);
            return;
        }

        const links = alertsList.querySelectorAll(".alert-symbol");
        let matchedLink = null;

        links.forEach(candidate => {
            if (!matchedLink && candidate.dataset.tvSymbol === chartActiveSymbol) {
                matchedLink = candidate;
            }
        });

        chartActiveLink = matchedLink;
        setActiveSymbolLink(matchedLink);
    }

    if (chartPanelCloseButton) {
        chartPanelCloseButton.addEventListener("click", () => {
            closeChartPanel();
        });
    }

    document.addEventListener("keydown", handleDocumentKeydown);

    function updateStatus(state, text) {
        statusDiv.textContent = text;
        statusDiv.className = `status ${state}`;
    }

    function connect() {
        updateStatus("status-connecting", "Связываемся с потоком сигналов...");
        const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        const wsUrl = `${wsProtocol}//${window.location.host}/ws/alerts`;

        socket = new WebSocket(wsUrl);

        socket.onopen = function() {
            updateStatus("status-connected", "На связи: получаем новые сигналы.");
        };

        socket.onmessage = function(event) {
            try {
                const alertData = JSON.parse(event.data);
                displayAlert(alertData);

                alertSound.play()
                    .catch(error => {
                        console.error("!!!!! ОСНОВНОЙ СКРИПТ: alertSound.play() НЕ УДАЛОСЬ:", error.name, error.message);

                        if (!document.getElementById('enableSoundButton')) {
                            const enableSoundButton = document.createElement('button');
                            enableSoundButton.id = 'enableSoundButton';
                            enableSoundButton.textContent = 'Разрешить звук сигналов';
                            enableSoundButton.onclick = () => {
                                alertSound.play().then(() => {
                                    enableSoundButton.style.display = 'none';
                                }).catch(e => console.error("ОСНОВНОЙ СКРИПТ: Ошибка при активации звука пользователем через кнопку:", e));
                            };

                            if (statusDiv && statusDiv.parentNode) {
                                statusDiv.parentNode.insertBefore(enableSoundButton, statusDiv.nextSibling);
                            } else {
                                document.body.appendChild(enableSoundButton); // Запасной вариант
                            }
                        }
                    });
            } catch (e) {
                console.error("ОСНОВНОЙ СКРИПТ: Ошибка парсинга JSON или в displayAlert:", e);
            }
        };

        socket.onerror = function(error) {
            console.error("ОСНОВНОЙ СКРИПТ: WebSocket onerror:", error);
            updateStatus("status-disconnected", "Что-то помешало подключению. Пробуем снова...");
        };

        socket.onclose = function(event) {
            console.log("ОСНОВНОЙ СКРИПТ: WebSocket onclose. Код:", event.code, "Причина:", event.reason, "Было чисто:", event.wasClean);
            updateStatus("status-disconnected", "Потеряли связь. Переподключимся через 5 секунд...");
            setTimeout(connect, 5000);
        };
    }

    function displayAlert(data) {
        if (!alertsList) { return; }
        if (!data || !data.symbol) { return; }

        const listItem = document.createElement("li");
        listItem.classList.add("alert");
        listItem.setAttribute("role", "button");
        listItem.setAttribute("tabindex", "0");

        const eventTime = new Date(data.timestamp).toLocaleString('ru-RU');

        const originalBinanceTicker = data.symbol;
        const tickerForTradingViewUrl = `${originalBinanceTicker}.P`;
        const tradingViewSymbolParam = `BINANCE:${tickerForTradingViewUrl}`;
        const tradingViewUrl = `https://www.tradingview.com/chart/?symbol=${encodeURIComponent(tradingViewSymbolParam)}`;

        let movementText = "";
        let movementClass = "";

        if (data.percentageChange > 0) {
            movementText = `выросла на <strong>${data.percentageChange.toFixed(2)}%</strong>`;
            movementClass = "price-rise";
        } else if (data.percentageChange < 0) {
            movementText = `упала на <strong>${Math.abs(data.percentageChange).toFixed(2)}%</strong>`;
            movementClass = "price-fall";
        } else {
            movementText = "осталась без изменений";
            movementClass = "price-neutral";
        }

        listItem.classList.add(movementClass);

        listItem.innerHTML = `
            <div class="alert-header">
                <span class="alert-title">Свежий сигнал</span>
                <a href="${tradingViewUrl}" target="_blank" class="alert-symbol" title="Открыть ${tickerForTradingViewUrl} на TradingView">${originalBinanceTicker}</a>
                <span class="alert-time">${eventTime}</span>
            </div>
            <div>
                Цена ${movementText} за последние 60 секунд.
            </div>
            <div class="alert-details">
                <span>Сейчас: <strong>${data.currentPrice.toFixed(4)}</strong></span>
                <span>Минуту назад: <strong>${data.oldestPrice.toFixed(4)}</strong></span>
            </div>
        `;

        const symbolLink = listItem.querySelector(".alert-symbol");
        if (symbolLink) {
            symbolLink.dataset.tvSymbol = tradingViewSymbolParam;
            symbolLink.dataset.rawSymbol = originalBinanceTicker;
            symbolLink.addEventListener("click", handleSymbolClick);
            listItem.setAttribute("aria-label", `Открыть график ${originalBinanceTicker}`);
            listItem.title = `Открыть график ${originalBinanceTicker}`;
        }

        listItem.addEventListener("click", handleAlertCardClick);
        listItem.addEventListener("keydown", handleAlertCardKeydown);

        alertsList.prepend(listItem);

        while (alertsList.children.length > 50) {
            const lastChild = alertsList.lastChild;
            alertsList.removeChild(lastChild);
        }

        if (chartActiveSymbol) {
            syncChartPanelLink();
        }
    }

    connect();
});
