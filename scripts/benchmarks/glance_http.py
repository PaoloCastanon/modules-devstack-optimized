#!/usr/bin/env python3
"""Streaming Glance load primitives; credentials and payloads never enter samples."""
import hashlib
import http.client
import json
from pathlib import Path
import ssl
import time
from urllib.parse import urlsplit

CHUNK = 64 * 1024
MAX_JSON = 2 * 1024**2


def payload(path, size, seed=b'glance-benchmark-v2'):
    """Create bounded deterministic test bytes once, outside measurement."""
    if type(size) is not int or not 0 < size <= 64 * 1024**2:
        raise ValueError('unsupported controlled payload size')
    hashes = {name: hashlib.new(name) for name in ['sha256', 'sha512', 'md5']}
    with Path(path).open('xb') as stream:
        remaining = size
        counter = 0
        while remaining:
            block = hashlib.shake_256(seed + counter.to_bytes(8, 'big')).digest(CHUNK)
            data = block[:min(remaining, CHUNK)]
            stream.write(data)
            for digest in hashes.values():
                digest.update(data)
            remaining -= len(data)
            counter += 1
    return {'size': size, **{name: digest.hexdigest() for name, digest in hashes.items()}}


class Client:
    def __init__(self, url, token, timeout=10, binary_timeout=120):
        parsed = urlsplit(url)
        if (parsed.scheme not in ('http', 'https') or not parsed.hostname or parsed.username
                or parsed.password or parsed.query or parsed.fragment or '%' in parsed.path):
            raise ValueError('plain trusted endpoint required')
        self.url, self.base = parsed, parsed.path.rstrip('/')
        self.token, self.timeout, self.binary_timeout = token, timeout, binary_timeout
        self.connection = None
        self.connection_timeout = None

    def close(self):
        if self.connection:
            self.connection.close()
        self.connection = None

    def request(self, method, path, *, body=None, content_type='application/json', upload=None, expected_payload=None):
        if not path.startswith('/v2/') or '\r' in path or '\n' in path or path.startswith('//'):
            raise ValueError('controlled Glance path required')
        timeout = self.binary_timeout if upload is not None or expected_payload is not None else self.timeout
        if self.connection is None:
            cls = http.client.HTTPSConnection if self.url.scheme == 'https' else http.client.HTTPConnection
            kwargs = {'timeout': timeout}
            if self.url.scheme == 'https':
                kwargs['context'] = ssl.create_default_context()
            self.connection = cls(self.url.hostname, self.url.port, **kwargs)
        else:
            self.connection.timeout = timeout
            if self.connection.sock is not None:
                self.connection.sock.settimeout(timeout)
        self.connection_timeout = timeout
        headers = {'X-Auth-Token': self.token, 'Content-Type': content_type}
        encoded = json.dumps(body).encode() if body is not None else None
        stream = None
        if upload is not None:
            stream = Path(upload).open('rb')
            headers['Content-Length'] = str(Path(upload).stat().st_size)
            encoded = stream
        start = time.monotonic_ns()
        status, count, first, value, error = None, 0, None, None, None
        try:
            self.connection.request(method, self.base + path, body=encoded, headers=headers)
            response = self.connection.getresponse()
            status = response.status
            if expected_payload is not None and status == 200:
                digest = hashlib.sha256()
                data = response.read(1)
                if data:
                    first = time.monotonic_ns()
                    count = 1
                    digest.update(data)
                while True:
                    data = response.read(CHUNK)
                    if not data:
                        break
                    count += len(data)
                    if count > expected_payload['size']:
                        raise ValueError('download size mismatch')
                    digest.update(data)
                if count != expected_payload['size'] or digest.hexdigest() != expected_payload['sha256']:
                    error = 'payload_mismatch'
            else:
                data = response.read(MAX_JSON + 1)
                first = time.monotonic_ns() if data else None
                count = len(data)
                if count > MAX_JSON:
                    raise ValueError('response exceeds bounded metadata size')
                if data and 200 <= status < 300:
                    try:
                        value = json.loads(data)
                    except (ValueError, UnicodeDecodeError):
                        error = 'invalid_json'
        except (OSError, http.client.HTTPException):
            error = 'transport'
            self.close()
        except ValueError:
            error = 'invalid_response'
            self.close()
        finally:
            if stream:
                stream.close()
        finish = time.monotonic_ns()
        return {'begin_ns': start, 'finish_ns': finish, 'elapsed_ns': finish - start,
                'ttfb_ns': None if first is None or expected_payload is None or status != 200 else first - start, 'status': status,
                'response_bytes': count, 'error_kind': error}, value


def metadata_matches(image, expected):
    """Selected contract, with strict scalar types; never normalize image state."""
    if not isinstance(image, dict):
        return False
    for name, value in expected.items():
        actual = image.get(name)
        if type(actual) is not type(value):
            return False
        if name == 'tags':
            if any(type(tag) is not str for tag in actual + value):
                return False
            if sorted(actual) != sorted(value):
                return False
        elif actual != value:
            return False
    return True
