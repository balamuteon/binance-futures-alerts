const RECONNECT_DELAY_MS = 5000;

function createAlertsUrl() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}/ws/alerts`;
}

export class AlertsSocket {
    constructor({ onConnecting, onOpen, onMessage, onError, onClose }) {
        this.onConnecting = onConnecting;
        this.onOpen = onOpen;
        this.onMessage = onMessage;
        this.onError = onError;
        this.onClose = onClose;
        this.socket = null;
        this.reconnectTimer = null;
    }

    connect() {
        window.clearTimeout(this.reconnectTimer);
        this.onConnecting?.();
        this.socket = new WebSocket(createAlertsUrl());

        this.socket.addEventListener("open", () => this.onOpen?.());
        this.socket.addEventListener("message", event => {
            try {
                this.onMessage?.(JSON.parse(event.data));
            } catch (error) {
                console.error("Не удалось обработать сообщение WebSocket:", error);
            }
        });
        this.socket.addEventListener("error", event => this.onError?.(event));
        this.socket.addEventListener("close", event => {
            this.onClose?.(event);
            this.reconnectTimer = window.setTimeout(() => this.connect(), RECONNECT_DELAY_MS);
        });
    }
}
