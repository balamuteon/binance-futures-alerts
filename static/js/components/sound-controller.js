export class SoundController {
    constructor({ statusElement, testButton, testPlayer, alertUrl }) {
        this.statusElement = statusElement;
        this.testPlayer = testPlayer;
        this.alertSound = new Audio(alertUrl);
        this.alertSound.preload = "auto";

        testButton?.addEventListener("click", () => this.playTest());
    }

    playTest() {
        if (!this.testPlayer) {
            console.error("Тестовый аудиоплеер не найден.");
            return;
        }

        this.testPlayer.play()
            .then(() => window.alert("Если все хорошо, вы услышали короткий сигнал."))
            .catch(error => {
                console.error("Не удалось воспроизвести тестовый звук:", error);
                window.alert(`Не удалось воспроизвести звук: ${error.name} — ${error.message}`);
            });
    }

    playAlert() {
        this.alertSound.play().catch(error => {
            console.error("Не удалось автоматически воспроизвести звук:", error);
            this.ensureEnableButton();
        });
    }

    ensureEnableButton() {
        if (document.getElementById("enableSoundButton")) {
            return;
        }

        const button = document.createElement("button");
        button.id = "enableSoundButton";
        button.type = "button";
        button.textContent = "Разрешить звук сигналов";
        button.addEventListener("click", () => {
            this.alertSound.play()
                .then(() => button.remove())
                .catch(error => console.error("Не удалось активировать звук:", error));
        });

        if (this.statusElement?.parentNode) {
            this.statusElement.parentNode.insertBefore(button, this.statusElement.nextSibling);
        } else {
            document.body.appendChild(button);
        }
    }
}
