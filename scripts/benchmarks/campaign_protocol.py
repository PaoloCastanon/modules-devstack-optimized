"""Protocol-derived accounting and inexpensive host storage monitoring."""
from pathlib import Path
import shutil
import threading


def accounting(protocol):
    scenarios, concurrency = protocol['scenarios'], protocol['concurrency']
    repetitions = protocol['repetitions']
    if (not scenarios or len(set(scenarios)) != len(scenarios)
            or not concurrency or len(set(concurrency)) != len(concurrency)
            or any(type(c) is not int or c not in (1, 4, 8, 16) for c in concurrency)
            or type(repetitions) is not int or repetitions < 3
            or not 30 <= protocol['measurement_s'] <= 3600
            or not 10 <= protocol['warmup_s'] <= 3600):
        raise ValueError('invalid official protocol matrix or measurement duration')
    count = 2 * len(scenarios) * len(concurrency) * repetitions
    if protocol.get('required_official_windows', count) != count:
        raise ValueError('declared window count disagrees with protocol matrix')
    return {'protocol': protocol['protocol'], 'runs': count, 'repetitions': repetitions,
            'concurrency': concurrency, 'warmup_s': protocol['warmup_s'],
            'measurement_s': protocol['measurement_s'],
            'scope': protocol.get('purpose', protocol.get('scope', 'Placement evaluation'))}


def storage_check(path, minimum):
    available = shutil.disk_usage(path).free
    if available < minimum:
        raise ValueError('insufficient host disk space for controlled campaign')
    return available


class StorageWatch:
    """Poll outside request paths; workers only inspect an in-memory event."""
    def __init__(self, path, minimum, interval=2):
        self.path, self.minimum, self.interval = Path(path), minimum, interval
        self.stop, self.failed = threading.Event(), threading.Event()
        self.minimum_observed = storage_check(self.path, minimum)
        self.thread = threading.Thread(target=self._run, daemon=True)

    def _run(self):
        while not self.stop.wait(self.interval):
            try:
                self.minimum_observed = min(self.minimum_observed, shutil.disk_usage(self.path).free)
                if self.minimum_observed < self.minimum:
                    self.failed.set()
            except OSError:
                self.failed.set()

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_):
        self.stop.set()
        self.thread.join()

    def report(self):
        return {'filesystem_path': str(self.path), 'minimum_required_bytes': self.minimum,
                'minimum_observed_free_bytes': self.minimum_observed,
                'aborted': self.failed.is_set(), 'poll_interval_s': self.interval}
