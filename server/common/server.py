import socket
import logging
import threading

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

        # sockets esperando winners: list[(sock, agency)]
        self._pending_qwin = []

        self._state_lock = threading.Lock()
        self._workers: set[threading.Thread] = set()

    def stop(self):
        self._shutdown = True
        try:
            self._server_socket.close()
        except Exception:
            pass

        with self._state_lock:
            pending = list(self._pending_qwin)
            self._pending_qwin.clear()

        for sock, _agency in pending:
            try:
                sock.close()
            except Exception:
                pass

        for t in list(self._workers):
            try:
                t.join(timeout=1.0)
            except Exception:
                pass

    def __reply_winners(self, client_sock, agency: int):
        bets = load_bets()
        winners = []
        for b in bets:
            if int(b.agency) != agency:
                continue
            if has_won(b):
                winners.append(str(b.document))
        send_frame(client_sock, encode_winners(winners))

    def __maybe_finish_draw_locked(self):
        if (not self._sorteo_done) and len(self._seen) > 0 and self._seen.issubset(self._finished):
            self._sorteo_done = True
            logging.info("action: sorteo | result: success")

            pending = list(self._pending_qwin)
            self._pending_qwin.clear()

        else:
            pending = []

        return pending

    def run(self):
        while not self._shutdown:
            try:
                client_sock = self.__accept_new_connection()
            except OSError:
                if self._shutdown:
                    break
                raise

            t = threading.Thread(target=self.__worker, args=(client_sock,), daemon=True)
            with self._state_lock:
                self._workers.add(t)
            t.start()

        logging.info("action: shutdown | result: success | component: server")

    def __worker(self, client_sock):
        try:
            self.__handle_client_connection(client_sock)
        finally:
            with self._state_lock:
                try:
                    self._workers.remove(threading.current_thread())
                except KeyError:
                    pass

    def __accept_new_connection(self):
        logging.info("action: accept_connections | result: in_progress")
        c, addr = self._server_socket.accept()
        logging.info(f"action: accept_connections | result: success | ip: {addr[0]}")
        return c

    def __handle_client_connection(self, client_sock):
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

                with self._state_lock:
                    self._seen.add(agency)
                    self._finished.add(agency)
                    pending = self.__maybe_finish_draw_locked()

                send_frame(client_sock, encode_ack(True))
                client_sock.close()
                for sock, ag in pending:
                    try:
                        self.__reply_winners(sock, ag)
                    except Exception:
                        try:
                            send_frame(sock, encode_ack(False))
                        except Exception:
                            pass
                    finally:
                        try:
                            sock.close()
                        except Exception:
                            pass
                return

            if msg_type == MSG_GET_WINNERS:
                action = "get_winners"
                if len(payload) < 2:
                    raise ValueError("GET_WINNERS payload too short")
                agency = int(payload[1])

                with self._state_lock:
                    if not self._sorteo_done:
                        self._pending_qwin.append((client_sock, agency))
                        return 

                self.__reply_winners(client_sock, agency)
                return

            # BET/BATCH
            action = "apuesta_recibida"
            msg = decode_message(payload)
            bets_in = msg["bets"]
            cantidad = len(bets_in)

            bets = []
            for b in bets_in:
                agency = int(b["agency"])
                with self._state_lock:
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
            return

        except Exception as e:
            logging.error(f"action: {action} | result: fail | error: {e}")
            try:
                send_frame(client_sock, encode_ack(False))
            except Exception:
                pass
        finally:
            try:
                with self._state_lock:
                    is_pending = any(sock is client_sock for sock, _ in self._pending_qwin)
                if not is_pending:
                    client_sock.close()
            except Exception:
                pass
