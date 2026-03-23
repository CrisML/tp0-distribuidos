import socket
import logging
import threading
import time

from common.protocol import (
    recv_frame,
    send_frame,
    decode_message,
    encode_ack,
    MSG_FIN,
    MSG_GET_WINNERS,
)
from common.utils import Bet, store_bets, load_bets, has_won


class Server:
    def __init__(self, port, listen_backlog):
        self._shutdown_event = threading.Event()

        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(("", port))
        self._server_socket.listen(listen_backlog)

        self._lock = threading.Lock()
        self._cv = threading.Condition(self._lock)
        self._seen = set()
        self._finished = set()
        self._sorteo_done = False
        self._workers = []

    def stop(self):
        self._shutdown_event.set()
        try:
            self._server_socket.close()
        except Exception:
            pass
        with self._cv:
            self._cv.notify_all()

    def __maybe_finish_draw_locked(self):
        if (not self._sorteo_done) and len(self._seen) > 0 and self._seen.issubset(self._finished):
            self._sorteo_done = True
            logging.info("action: sorteo | result: success")
            self._cv.notify_all()

    def run(self):
        while not self._shutdown_event.is_set():
            try:
                logging.info("action: accept_connections | result: in_progress")
                client_sock, addr = self._server_socket.accept()
                logging.info(f"action: accept_connections | result: success | ip: {addr[0]}")
            except OSError:
                break

            t = threading.Thread(target=self.__handle_client_connection, args=(client_sock,), daemon=True)
            self._workers.append(t)
            t.start()

        # opcional: esperar a que terminen threads activas un ratito
        deadline = time.time() + 2.0
        for t in self._workers:
            remaining = deadline - time.time()
            if remaining <= 0:
                break
            t.join(timeout=remaining)

        logging.info("action: shutdown | result: success | component: server")

    def __handle_client_connection(self, client_sock):
        cantidad = 0
        action = "unknown"
        try:
            payload = recv_frame(client_sock)
            if len(payload) < 1:
                raise ValueError("empty payload")

            msg_type = payload[0]

            if msg_type == MSG_FIN:
                action = "fin"
                if len(payload) < 2:
                    raise ValueError("FIN payload too short")
                agency = int(payload[1])
                with self._cv:
                    self._seen.add(agency)
                    self._finished.add(agency)
                    self.__maybe_finish_draw_locked()
                send_frame(client_sock, encode_ack(True))
                return

            if msg_type == MSG_GET_WINNERS:
                action = "get_winners"
                if len(payload) < 2:
                    raise ValueError("GET_WINNERS payload too short")
                agency = int(payload[1])

                with self._cv:
                    deadline = time.time() + 290
                    while not self._sorteo_done:
                        remaining = deadline - time.time()
                        if remaining <= 0:
                            break
                        self._cv.wait(timeout=remaining)

                    if not self._sorteo_done:
                        send_frame(client_sock, encode_ack(False))
                        return

                bets = load_bets()
                winners = []
                for b in bets:
                    if int(b.agency) != agency:
                        continue
                    if has_won(b):
                        winners.append(str(b.document))

                from common.protocol import encode_winners
                send_frame(client_sock, encode_winners(winners))
                return

            action = "apuesta_recibida"
            msg = decode_message(payload)
            bets_in = msg["bets"]
            cantidad = len(bets_in)

            bets = []
            for b in bets_in:
                agency = int(b["agency"])
                with self._cv:
                    self._seen.add(agency)

                bets.append(
                    Bet(
                        str(b["agency"]),
                        b["first_name"],
                        b["last_name"],
                        b["document"],
                        b["birthdate"],
                        str(b["number"]),
                    )
                )

            store_bets(bets)
            logging.info(f"action: apuesta_recibida | result: success | cantidad: {cantidad}")
            send_frame(client_sock, encode_ack(True))

        except Exception as e:
            if action == "apuesta_recibida":
                logging.error(f"action: apuesta_recibida | result: fail | cantidad: {cantidad}")
            else:
                logging.error(f"action: {action} | result: fail | error: {e}")
            try:
                send_frame(client_sock, encode_ack(False))
            except Exception:
                pass
        finally:
            try:
                client_sock.close()
            except Exception:
                pass
