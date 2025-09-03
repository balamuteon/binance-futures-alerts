// load_test.js - Исправленный сценарий

import ws from 'k6/ws';
import { check } from 'k6';
import { sleep } from 'k6';

export const options = {
  stages: [
    // Плавно наращиваем нагрузку до 100 пользователей за 30 секунд
    { duration: '10s', target: 100 },
    // Держим 100 пользователей в течение 1 минуты
    { duration: '1m', target: 10000 },
    // Плавно снижаем до 0 за 10 секунд
    { duration: '1m', target: 20000 },
    { duration: '1m', target: 20000 },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    'checks': ['rate>0.99'], // 99% проверок должны быть успешными
    'ws_session_duration': ['p(95)<120000'], // 95% сессий должны длиться менее 120 секунд
  },
};

export default function () {
  const url = 'ws://localhost:8080/ws/alerts';

  // ws.connect является блокирующим вызовом, пока колбэк-функция не завершится
  const res = ws.connect(url, null, function (socket) {
    // 1. Код внутри этой функции выполняется ПОСЛЕ успешного установления соединения
    socket.on('open', () => {
      console.log(`VU ${__VU}: WebSocket connection established!`);
      // Можно отправлять какие-то данные при подключении, если нужно
      // socket.send(JSON.stringify({ event: 'subscribe', channel: 'alerts' }));
    });

    // 2. Обрабатываем входящие сообщения
    socket.on('message', (data) => {
      // console.log(`VU ${__VU}: Received message: ${data}`);
      // Здесь можно добавить проверки для содержимого сообщения, если это необходимо
      check(data, {
        'message is not empty': (d) => d.length > 0,
      });
    });

    // 3. Удерживаем соединение открытым в течение некоторого времени
    // Каждый VU будет держать соединение открытым ~30 секунд
    socket.setTimeout(() => {
      console.log(`VU ${__VU}: Closing connection after 30s.`);
      socket.close();
    }, 30000); // 30 секунд

    // 4. Обрабатываем закрытие соединения
    socket.on('close', () => {
      console.log(`VU ${__VU}: Connection closed.`);
    });

    // 5. Обрабатываем ошибки
    socket.on('error', function (e) {
      if (e.error() != 'websocket: close sent') {
        console.error(`VU ${__VU}: An unexpected error occured: ${e.error()}`);
      }
    });
  });

  // Эта проверка выполняется СРАЗУ после handshake (запроса на переключение протоколов)
  // и ДО того, как колбэк выше начнет выполняться в полной мере.
  check(res, { 'status is 101': (r) => r && r.status === 101 });
}