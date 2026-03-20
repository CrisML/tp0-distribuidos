import socket
import logging
import threading

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
            msg = client_sock.recv(1024).rstrip().decode('utf-8')
            addr = client_sock.getpeername()
            logging.info(f'action: receive_message | result: success | ip: {addr[0]} | msg: {msg}')
            client_sock.send("{}\n".format(msg).encode('utf-8'))
        except OSError as e:
            logging.error(f"action: receive_message | result: fail | error: {e}")
        finally:
            try:
                client_sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            client_sock.close()
            logging.info("action: close_fd | result: success | component: server | fd: client_socket")

    def __accept_new_connection(self):
        logging.info('action: accept_connections | result: in_progress')
        c, addr = self._server_socket.accept()
        logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
        return c
