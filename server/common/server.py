import socket
import logging
import threading
import time

from common.protocol import recv_frame, send_frame, decode_message, encode_ack, decode_simple, MSG_FIN, MSG_GET_WINNERS
from common.utils import Bet, store_bets, load_bets, has_won

class Server:
    def __init__(self, port, listen_backlog):
        self._shutdown_event = threading.Event()

        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)

        self._lock = threading.Lock()
        self._cv = threading.Condition(self._lock)
        self._finished = set()
        self._sorteo_done = False

    def stop(self):
        """Request graceful shutdown: stop accepting and close listen socket."""
        if self._shutdown_event.is_set():
            return

        self._shutdown_event.set()
        logging.info("action: shutdown | result: in_progress | component: server")

        try:
            try:
                self._server_socket.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            self._server_socket.close()
            logging.info("action: close_fd | result: success | component: server | fd: listen_socket")
        except Exception as e:
            logging.info(f"action: close_fd | result: fail | component: server | fd: listen_socket | error: {e}")

    def run(self):
        """
        Server loop: accepts connections until shutdown is requested.
        """
        while not self._shutdown_event.is_set():
            try:
                client_sock = self.__accept_new_connection()
            except OSError:
                # Expected path when listen socket is closed during shutdown
                if self._shutdown_event.is_set():
                    break
                raise

            self.__handle_client_connection(client_sock)

        logging.info("action: shutdown | result: success | component: server")

    def __handle_client_connection(self, client_sock):
        try:
            payload = recv_frame(client_sock)

            msg_type = payload[0] if len(payload) > 0 else None

            if msg_type == MSG_FIN:
                agency = payload[1]
                with self._cv:
                    self._finished.add(int(agency))
                    if len(self._finished) >= 5 and not self._sorteo_done:
                        self._sorteo_done = True
                        logging.info("action: sorteo | result: success")
                    self._cv.notify_all()

                send_frame(client_sock, encode_ack(True))
                return

            if msg_type == MSG_GET_WINNERS:
                agency = payload[1]
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
                    if str(b.agency) != str(int(agency)):
                        continue
                    if has_won(b):
                        winners.append(str(b.document))

                from common.protocol import encode_winners
                send_frame(client_sock, encode_winners(winners))
                return

            msg = decode_message(payload)
            bets_in = msg["bets"]
            cantidad = len(bets_in)

            bets = []
            for b in bets_in:
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

        except Exception:
            try:
                cantidad = cantidad if "cantidad" in locals() else 0
            except Exception:
                cantidad = 0
            logging.error(f"action: apuesta_recibida | result: fail | cantidad: {cantidad}")
            try:
                send_frame(client_sock, encode_ack(False))
            except Exception:
                pass
        finally:
            client_sock.close()

    def __accept_new_connection(self):
        logging.info('action: accept_connections | result: in_progress')
        c, addr = self._server_socket.accept()
        logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
        return c
