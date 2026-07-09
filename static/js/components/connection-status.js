const STATUS_CLASS_PREFIX = "status-";

export class ConnectionStatus {
    constructor(element) {
        this.element = element;
    }

    set(state, text) {
        if (!this.element) {
            return;
        }

        this.element.textContent = text;
        this.element.className = `status ${STATUS_CLASS_PREFIX}${state}`;
    }
}
