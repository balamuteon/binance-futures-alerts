import { createTradingViewEmbedUrl } from "../utils/trading-view.js";

export class ChartPanel {
    constructor({ appShell, panel, symbol, openLink, closeButton }) {
        this.appShell = appShell;
        this.panel = panel;
        this.symbol = symbol;
        this.openLink = openLink;
        this.closeButton = closeButton;
        this.frame = panel?.querySelector(".chart-panel__frame") ?? null;
        this.alertList = null;
        this.activeSymbol = null;

        this.closeButton?.addEventListener("click", () => this.close());
        document.addEventListener("keydown", event => {
            if (event.key === "Escape" && this.isOpen) {
                this.close();
            }
        });
    }

    get isOpen() {
        return this.panel?.getAttribute("aria-hidden") === "false";
    }

    setAlertList(alertList) {
        this.alertList = alertList;
    }

    open(link) {
        if (!this.panel || !this.openLink || !this.appShell || !link?.dataset.tvSymbol) {
            return;
        }

        const tradingViewSymbol = link.dataset.tvSymbol;
        this.ensureIframe(tradingViewSymbol);

        if (this.symbol) {
            const displaySymbol = link.dataset.rawSymbol || link.textContent?.trim() || tradingViewSymbol;
            this.symbol.textContent = displaySymbol;
            this.symbol.title = tradingViewSymbol;
        }

        this.panel.dataset.symbol = tradingViewSymbol;
        this.openLink.href = link.href;
        this.openLink.removeAttribute("tabindex");
        this.closeButton?.removeAttribute("tabindex");
        this.activeSymbol = tradingViewSymbol;
        this.setActiveLink(link);

        this.appShell.classList.add("app-shell--chart-open");
        this.panel.setAttribute("aria-hidden", "false");
    }

    close() {
        if (!this.panel || !this.appShell) {
            return;
        }

        this.panel.removeAttribute("data-symbol");
        this.panel.setAttribute("aria-hidden", "true");
        this.appShell.classList.remove("app-shell--chart-open");

        if (this.symbol) {
            this.symbol.textContent = "—";
            this.symbol.removeAttribute("title");
        }

        if (this.openLink) {
            this.openLink.href = "#";
            this.openLink.setAttribute("tabindex", "-1");
        }

        this.closeButton?.setAttribute("tabindex", "-1");
        this.activeSymbol = null;
        this.setActiveLink(null);
    }

    syncActiveLink() {
        if (!this.alertList || !this.activeSymbol) {
            this.setActiveLink(null);
            return;
        }

        const matchingLink = [...this.alertList.querySelectorAll(".alert-symbol")]
            .find(link => link.dataset.tvSymbol === this.activeSymbol);
        this.setActiveLink(matchingLink ?? null);
    }

    ensureIframe(symbol) {
        if (!this.frame) {
            return;
        }

        let iframe = this.frame.querySelector("iframe");
        if (!iframe) {
            iframe = document.createElement("iframe");
            iframe.setAttribute("allowtransparency", "true");
            iframe.setAttribute("scrolling", "no");
            iframe.loading = "lazy";
            iframe.title = "TradingView preview";
            iframe.referrerPolicy = "no-referrer-when-downgrade";
            this.frame.appendChild(iframe);
        }

        if (iframe.dataset.loadedSymbol !== symbol) {
            iframe.src = createTradingViewEmbedUrl(symbol);
            iframe.dataset.loadedSymbol = symbol;
        }
    }

    setActiveLink(link) {
        if (!this.alertList) {
            return;
        }

        this.alertList.querySelectorAll(".alert-symbol--active").forEach(activeLink => {
            if (activeLink !== link) {
                activeLink.classList.remove("alert-symbol--active");
            }
        });
        link?.classList.add("alert-symbol--active");
    }
}
