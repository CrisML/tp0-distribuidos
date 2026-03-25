import socket
import logging
import time

from common.protocol import (
    recv_frame,
    send_frame,
    decode_message,
    encode_ack,
    MSG_FIN,
    MSG_GET_WINNERS,
    encode_winners,
)
from common.utils import Bet, store_bets, load_bets, has_won


class Server:
    def __init__(self, port, listen_backlog):
        self._shutdown = False

        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind(("", port))
        self._server_socket.listen(listen_backlog)

        self._seen = set()
        self._finished = set()
        self._sorteo_done = False

        self._pending_qwin = []

    def stop(self):
        self._shutdown = True
        try:
            self._server_socket.close()
        except Exception:
            pass
        # cerrar pendientes
        for sock, _agency in list(self._pending_qwin):
            try:
                sock.close()
            except Exception:
                pass
        self._pending_qwin.clear()

    def __maybe_finish_draw(self):
        if (not self._sorteo_done) and len(self._seen) > 0 and self._seen.issubset(self._finished):
            self._sorteo_done = True
            logging.info("action: sorteo | result: success")

            pending = list(self._pending_qwin)
            self._pending_qwin.clear()
            for sock, agency in pending:
                try:
                    self.__reply_winners(sock, agency)
                except Exception as e:
                    logging.error(f"action: get_winners | result: fail | error: {e}")
                    try:
                        send_frame(sock, encode_ack(False))
                    except Exception:
                        pass
                finally:
                    try:
                        sock.close()
                    except Exception:
                        pass

    def run(self):
        while not self._shutdown:
            try:
                client_sock = self.__accept_new_connection()
            except OSError:
                if self._shutdown:
                    break
                raise

            self.__handle_client_connection(client_sock)

        logging.info("action: shutdown | result: success | component: server")

    def __accept_new_connection(self):
        logging.info("action: accept_connections | result: in_progress")
        c, addr = self._server_socket.accept()
        logging.info(f"action: accept_connections | result: success | ip: {addr[0]}")
        return c

    def __reply_winners(self, client_sock, agency: int):
        bets = load_bets()
        winners = []
        for b in bets:
            if int(b.agency) != agency:
                continue
            if has_won(b):
                winners.append(str(b.document))
        send_frame(client_sock, encode_winners(winners))

    def __handle_client_connection(self, client_sock):
        action = "unknown"
        cantidad = 0
        should_close = True
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

                self._seen.add(agency)
                self._finished.add(agency)
                self.__maybe_finish_draw()

                send_frame(client_sock, encode_ack(True))
                return

            if msg_type == MSG_GET_WINNERS:
                action = "get_winners"
                if len(payload) < 2:
                    raise ValueError("GET_WINNERS payload too short")
                agency = int(payload[1])

                if not self._sorteo_done:
                    with self._cv:
                        self._pending_qwin.append((client_sock, agency))
                    return

                send_frame(client_sock, encode_ack(False))
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
            should_close = False

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
            if should_close:
                try:
                    client_sock.close()
                except Exception:
                    pass
