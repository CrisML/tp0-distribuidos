import csv
import datetime
import threading


""" Bets storage location. """
STORAGE_FILEPATH = "./bets.csv"
""" Simulated winner number in the lottery contest. """
LOTTERY_WINNER_NUMBER = 7574

_storage_lock = threading.Lock()


""" A lottery bet registry. """
class Bet:
    def __init__(self, agency: str, first_name: str, last_name: str, document: str, birthdate: str, number: str):
        """
        agency must be passed with integer format.
        birthdate must be passed with format: 'YYYY-MM-DD'.
        number must be passed with integer format.
        """
        self.agency = int(agency)
        self.first_name = first_name
        self.last_name = last_name
        self.document = document
        self.birthdate = datetime.date.fromisoformat(birthdate)
        self.number = int(number)

""" Checks whether a bet won the prize or not. """
def has_won(bet: Bet) -> bool:
    return bet.number == LOTTERY_WINNER_NUMBER

"""
Persist the information of each bet in the STORAGE_FILEPATH file.
Not thread-safe/process-safe.
"""
def store_bets(bets: list[Bet]) -> None:
    with _storage_lock:
        with open(STORAGE_FILEPATH, 'a+', newline='') as file:
            writer = csv.writer(file, quoting=csv.QUOTE_MINIMAL)
            for bet in bets:
                writer.writerow([bet.agency, bet.first_name, bet.last_name,
                                 bet.document, bet.birthdate, bet.number])
            file.flush()

"""
Loads the information all the bets in the STORAGE_FILEPATH file.
Not thread-safe/process-safe.
"""
def load_bets() -> list[Bet]:
    with _storage_lock:
        out: list[Bet] = []
        with open(STORAGE_FILEPATH, 'r', newline='') as file:
            reader = csv.reader(file, quoting=csv.QUOTE_MINIMAL)
            for row in reader:
                out.append(Bet(row[0], row[1], row[2], row[3], row[4], row[5]))
        return out

