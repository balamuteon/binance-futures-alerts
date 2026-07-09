import { isModifiedPointerEvent } from "../utils/events.js";
import { createTradingViewLink } from "../utils/trading-view.js";

const MAX_ALERTS = 50;

function appendTextWithStrong(container, prefix, strongText, suffix = "") {
    container.append(prefix);
    const strong = document.createElement("strong");
    strong.textContent = strongText;
    container.append(strong, suffix);
}

function getMovement(percentageChange) {
    if (percentageChange > 0) {
        return {
            className: "price-rise",
            prefix: "Цена выросла на ",
            value: `${percentageChange.toFixed(2)}%`,
        };
    }

    if (percentageChange < 0) {
        return {
            className: "price-fall",
            prefix: "Цена упала на ",
            value: `${Math.abs(percentageChange).toFixed(2)}%`,
        };
    }

    return {
        className: "price-neutral",
        prefix: "Цена осталась без изменений",
        value: null,
    };
}

function createAlertCard(data) {
    const { chartUrl, displayTicker, tradingViewSymbol } = createTradingViewLink(data.symbol);
    const movement = getMovement(data.percentageChange);
    const listItem = document.createElement("li");
    listItem.className = `alert ${movement.className}`;
    listItem.setAttribute("role", "button");
    listItem.setAttribute("tabindex", "0");
    listItem.setAttribute("aria-label", `Открыть график ${data.symbol}`);
    listItem.title = `Открыть график ${data.symbol}`;

    const header = document.createElement("div");
    header.className = "alert-header";

    const title = document.createElement("span");
    title.className = "alert-title";
    title.textContent = "Свежий сигнал";

    const symbol = document.createElement("a");
    symbol.className = "alert-symbol";
    symbol.href = chartUrl;
    symbol.target = "_blank";
    symbol.rel = "noopener";
    symbol.title = `Открыть ${displayTicker} на TradingView`;
    symbol.textContent = data.symbol;
    symbol.dataset.tvSymbol = tradingViewSymbol;
    symbol.dataset.rawSymbol = data.symbol;

    const time = document.createElement("span");
    time.className = "alert-time";
    time.textContent = new Date(data.timestamp).toLocaleString("ru-RU");
    header.append(title, symbol, time);

    const movementText = document.createElement("div");
    if (movement.value) {
        appendTextWithStrong(movementText, movement.prefix, movement.value, " за последние 60 секунд.");
    } else {
        movementText.textContent = `${movement.prefix} за последние 60 секунд.`;
    }

    const details = document.createElement("div");
    details.className = "alert-details";
    const currentPrice = document.createElement("span");
    const previousPrice = document.createElement("span");
    appendTextWithStrong(currentPrice, "Сейчас: ", data.currentPrice.toFixed(4));
    appendTextWithStrong(previousPrice, "Минуту назад: ", data.oldestPrice.toFixed(4));
    details.append(currentPrice, previousPrice);

    listItem.append(header, movementText, details);
    return listItem;
}

export class AlertList {
    constructor(element, { onOpenChart, onChange } = {}) {
        this.element = element;
        this.onOpenChart = onOpenChart;
        this.onChange = onChange;

        this.element?.addEventListener("click", event => this.handleClick(event));
        this.element?.addEventListener("keydown", event => this.handleKeydown(event));
    }

    add(data) {
        if (!this.element || !data?.symbol) {
            return false;
        }

        try {
            this.element.prepend(createAlertCard(data));

            while (this.element.children.length > MAX_ALERTS) {
                this.element.lastElementChild?.remove();
            }

            this.onChange?.();
            return true;
        } catch (error) {
            console.error("Не удалось отобразить сигнал:", error, data);
            return false;
        }
    }

    handleClick(event) {
        const link = event.target.closest(".alert-symbol");
        if (link && this.element.contains(link)) {
            event.stopPropagation();
            if (isModifiedPointerEvent(event)) {
                return;
            }

            event.preventDefault();
            this.onOpenChart?.(link);
            return;
        }

        const card = event.target.closest(".alert");
        if (!card || !this.element.contains(card) || isModifiedPointerEvent(event)) {
            return;
        }

        event.preventDefault();
        this.openCardChart(card);
    }

    handleKeydown(event) {
        if (event.key !== "Enter" && event.key !== " ") {
            return;
        }

        const card = event.target.closest(".alert");
        if (!card || !this.element.contains(card)) {
            return;
        }

        event.preventDefault();
        this.openCardChart(card);
    }

    openCardChart(card) {
        const link = card.querySelector(".alert-symbol");
        if (link) {
            this.onOpenChart?.(link);
        }
    }
}
