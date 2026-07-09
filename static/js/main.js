import { AlertList } from "./components/alert-list.js";
import { ChartPanel } from "./components/chart-panel.js";
import { ConnectionStatus } from "./components/connection-status.js";
import { SoundController } from "./components/sound-controller.js";
import { AlertsSocket } from "./services/alerts-socket.js";

function initializeApp() {
    const status = new ConnectionStatus(document.getElementById("status"));
    const chartPanel = new ChartPanel({
        appShell: document.querySelector(".app-shell"),
        panel: document.getElementById("chartPanel"),
        symbol: document.getElementById("chartPanelSymbol"),
        openLink: document.getElementById("chartPanelOpen"),
        closeButton: document.getElementById("chartPanelClose"),
    });
    const alertList = new AlertList(document.getElementById("alertsList"), {
        onOpenChart: link => chartPanel.open(link),
        onChange: () => chartPanel.syncActiveLink(),
    });
    const sound = new SoundController({
        statusElement: status.element,
        testButton: document.getElementById("testSoundButton"),
        testPlayer: document.getElementById("testAudioPlayer"),
        alertUrl: "assets/alert.wav",
    });

    chartPanel.setAlertList(alertList.element);

    const alertsSocket = new AlertsSocket({
        onConnecting: () => {
            status.set("connecting", "Связываемся с потоком сигналов...");
        },
        onOpen: () => {
            status.set("connected", "На связи: получаем новые сигналы.");
        },
        onMessage: data => {
            if (alertList.add(data)) {
                sound.playAlert();
            }
        },
        onError: error => {
            console.error("Ошибка WebSocket:", error);
            status.set("disconnected", "Что-то помешало подключению. Пробуем снова...");
        },
        onClose: event => {
            console.info(
                "WebSocket закрыт. Код:",
                event.code,
                "Причина:",
                event.reason,
                "Было чисто:",
                event.wasClean,
            );
            status.set("disconnected", "Потеряли связь. Переподключимся через 5 секунд...");
        },
    });

    alertsSocket.connect();
}

document.addEventListener("DOMContentLoaded", initializeApp);
