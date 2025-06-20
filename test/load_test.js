// load_test.js - Сценарий для высокой нагрузки

import ws from 'k6/ws';
import { check } from 'k6';

// --- НОВЫЕ НАСТРОЙКИ ТЕСТА ---
export const options = {
  // Сценарий с выходом на 50,000 пользователей
  stages: [
    { duration: '1m', target: 5000 },    // За 1 минуту быстро доходим до 5k
    { duration: '2m', target: 20000 },   // За следующие 2 минуты добираемся до 20k
    // { duration: '3m', target: 50000 },   // За 3 минуты выходим на пик в 50k
    // { duration: '2m', target: 50000 },   // 2 минуты держим максимальную нагрузку
    // { duration: '1m', target: 0 },       // Плавно снижаем нагрузку
  ],
  // Порог для ошибок. Если ошибок будет больше 1%, тест прервется.
  thresholds: {
    'checks': ['rate>0.99'], // Проверки должны проходить в 99% случаев
  },
};

// --- Основная логика виртуального пользователя (остается без изменений) ---
export default function () {
  const url = 'ws://localhost:8080/ws/alerts';

  const res = ws.connect(url, null, function (socket) {
    socket.on('open', () => {
      // Соединение установлено
    });

    socket.on('message', (data) => {
      // Получаем алерты от сервера
    });

    socket.on('close', () => {
      // Соединение закрыто
    });

    // Поддерживаем соединение живым
    socket.setInterval(() => {
      socket.ping();
    }, 20000);
  });

  check(res, { 'status is 101': (r) => r && r.status === 101 });
}

