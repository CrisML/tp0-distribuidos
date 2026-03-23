import socket
import logging
import threading

from common.protocol import recv_frame, send_frame, decode_message, encode_ack
from common.utils import Bet, store_bets

class Server:
    def __init__(self, port, listen_backlog):
        self._shutdown_event = threading.Event()

        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)

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

            # Si store_bets lanza excepción por alguna apuesta, consideramos batch fail
            store_bets(bets)

            logging.info(f"action: apuesta_recibida | result: success | cantidad: {cantidad}")
            # (opcional) mantener log individual del ej5 si querés compatibilidad:
            for bet in bets:
                logging.info(f"action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}")

            send_frame(client_sock, encode_ack(True))
        except Exception as e:
            try:
                # si llegamos a parsear lo suficiente como para conocer cantidad:
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
