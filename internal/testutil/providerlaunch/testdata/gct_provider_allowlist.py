"""Strict provider policy parsing and on-disk integrity shared by ops tools."""

from __future__ import annotations

import hashlib
from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import stat
from typing import Any, Iterator

SCHEMA = 'gc.provider-allowlist.v1'
IDENTIFIER = re.compile(r'[A-Za-z0-9][A-Za-z0-9._+-]*\Z')
DIGEST = re.compile(r'[0-9a-f]{64}\Z')
MAX_BYTES = 1024 * 1024
CLOSED_VERSION = re.compile(
    r'(gct-claude-(?:signing|candidate|template-candidate|project-candidate)-worker)'
    r' 2 sources_sha256=([0-9a-f]{64}) cli_sha256=([0-9a-f]{64})\Z')


class AllowlistError(ValueError):
    """A policy or executable does not satisfy its declared integrity."""


def require(condition: bool, message: str) -> None:
    """Raise a contextual refusal instead of proceeding with partial checks."""
    if not condition:
        raise AllowlistError(message)


def identifier(value: object, label: str) -> str:
    """Validate a nonempty, single path-component identifier."""
    require(isinstance(value, str) and bool(IDENTIFIER.fullmatch(value)), f'invalid {label}')
    return str(value)


def absolute(value: object, label: str) -> Path:
    """Require a clean absolute filesystem path, without hidden normalization."""
    require(isinstance(value, str) and '\0' not in value and bool(value), f'invalid {label}')
    path = Path(str(value))
    require(path.is_absolute() and os.path.normpath(str(value)) == value
            and not str(value).startswith('//'), f'{label} must be a clean absolute path')
    return path


def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    """Reject duplicate fields rather than accepting last-value-wins JSON."""
    result: dict[str, Any] = {}
    for key, value in pairs:
        require(key not in result, f'duplicate JSON field {key!r}')
        result[key] = value
    return result


def read_regular(path: Path, limit: int = MAX_BYTES) -> bytes:
    """Read a bounded regular file without following its final symlink."""
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as stream:
            require(stat.S_ISREG(os.fstat(stream.fileno()).st_mode), f'{path}: not a regular file')
            data = stream.read(limit + 1)
        require(len(data) <= limit, f'{path}: exceeds {limit} bytes')
        return data
    except OSError as exc:
        raise AllowlistError(f'read {path}: {exc}') from exc


def load(path: Path) -> dict[str, Any]:
    """Load and validate the complete allowlist from a regular JSON file."""
    return parse(read_regular(path))


def parse(data: bytes) -> dict[str, Any]:
    """Decode exact JSON keys and types from one bounded byte image."""
    require(len(data) <= MAX_BYTES, 'allowlist exceeds 1 MiB')
    return validate(decode_json(data))


def decode_json(data: bytes) -> Any:
    """Decode UTF-8 without BOM, encoding autodetection or duplicate keys."""
    require(len(data) <= 16 * MAX_BYTES, 'JSON exceeds 16 MiB')
    def invalid_constant(value: str) -> None:
        """JSON has no non-finite number literals."""
        raise AllowlistError(f'invalid JSON constant {value}')
    try:
        document = json.loads(data.decode('utf-8'), object_pairs_hook=unique_object,
                              parse_constant=invalid_constant)
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError) as exc:
        raise AllowlistError(f'allowlist: invalid JSON: {exc}') from exc
    pending = [(document, 0)]
    while pending:
        value, depth = pending.pop()
        require(depth <= 64, 'JSON nesting exceeds 64')
        if isinstance(value, dict):
            pending.extend((child, depth + 1) for child in value.values())
            pending.extend((key, depth) for key in value)
        elif isinstance(value, list):
            pending.extend((child, depth + 1) for child in value)
        elif isinstance(value, str):
            require(not any(0xD800 <= ord(character) <= 0xDFFF for character in value),
                    'JSON contains an unpaired Unicode surrogate')
    return document


def _fold_key(key: str) -> str:
    """Match Go's simple case fold for the ASCII extension-key vocabulary."""
    return key.lower().replace('\u017f', 's')


def validate_optional(document: Any, *, manifest: bool = False) -> bool:
    """Refuse embedded aliases before treating a missing extension as legacy."""
    require(isinstance(document, dict), 'document must be an object')
    if manifest:
        require(not any(_fold_key(key) == 'integrity' and key != 'integrity' for key in document),
                'integrity key must be exact')
        document = document.get('integrity', {})
        if document is None:
            return False
        require(isinstance(document, dict), 'integrity must be an object')
    require(not any(_fold_key(key) == 'provider_allowlist' and key != 'provider_allowlist'
                    for key in document), 'provider_allowlist key must be exact')
    if 'provider_allowlist' not in document:
        return False
    ref = document['provider_allowlist']
    fields = {'path', 'bindings'} if manifest else {'path', 'sha256'}
    require(isinstance(ref, dict) and set(ref) == fields, 'invalid provider_allowlist fields')
    absolute(ref['path'], 'provider_allowlist path')
    if manifest:
        require(isinstance(ref['bindings'], list) and bool(ref['bindings']), 'invalid bindings')
        for binding in ref['bindings']:
            require(isinstance(binding, dict) and set(binding) == {'provider', 'path'},
                    'invalid provider binding fields')
            identifier(binding['provider'], 'binding provider')
            absolute(binding['path'], 'binding path')
    else:
        require(isinstance(ref['sha256'], str) and bool(DIGEST.fullmatch(ref['sha256'])),
                'invalid provider_allowlist sha256')
    return True


def validate(document: Any) -> dict[str, Any]:
    """Check the schema, one-active invariant and rollback references."""
    require(isinstance(document, dict), 'allowlist must be an object')
    require(set(document) in ({'schema', 'entries'}, {'schema', 'entries', 'previous'}),
            'allowlist has unknown or missing fields')
    require(document['schema'] == SCHEMA, 'unsupported provider allowlist schema')
    entries = document['entries']
    require(isinstance(entries, list), 'entries must be an array')
    seen: set[tuple[str, str]] = set()
    providers: set[str] = set()
    active: dict[str, str] = {}
    for entry in entries:
        require(isinstance(entry, dict) and set(entry) == {'provider', 'version', 'path', 'sha256', 'active'},
                'provider entry has unknown or missing fields')
        provider = identifier(entry['provider'], 'provider')
        version = identifier(entry['version'], 'version')
        absolute(entry['path'], 'provider path')
        require(isinstance(entry['sha256'], str) and bool(DIGEST.fullmatch(entry['sha256'])),
                'provider sha256 must be lowercase SHA-256')
        require(type(entry['active']) is bool, 'provider active must be boolean')
        require((provider, version) not in seen, f'duplicate provider/version {provider}/{version}')
        seen.add((provider, version))
        providers.add(provider)
        if entry['active']:
            require(provider not in active, f'duplicate active provider {provider}')
            active[provider] = version
    require(set(active) == providers, 'each provider must have exactly one active entry')
    previous = document.get('previous', {})
    require(isinstance(previous, dict), 'previous must be an object')
    for provider, version in previous.items():
        require(isinstance(version, str) and (provider, version) in seen,
                f'previous {provider} version is missing')
        require(active.get(provider) != version, f'previous {provider} must be inactive')
    return document


def digest(path: Path) -> str:
    """Stream an executable hash while refusing nonregular files and symlinks."""
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as stream:
            require(stat.S_ISREG(os.fstat(stream.fileno()).st_mode), f'{path}: not a regular file')
            return hashlib.file_digest(stream, 'sha256').hexdigest()
    except OSError as exc:
        raise AllowlistError(f'hash {path}: {exc}') from exc


def verify_entry(entry: dict[str, Any]) -> None:
    """Require exact bytes at a non-symlinked, read-only executable path."""
    path = absolute(entry['path'], 'provider path')
    try:
        require(path.resolve(strict=True) == path, f'provider {entry["provider"]}: immutable path is a symlink')
        info = path.lstat()
        require(stat.S_ISREG(info.st_mode), f'provider {entry["provider"]}: not a regular file')
        require(stat.S_IMODE(info.st_mode) == 0o555, f'provider {entry["provider"]}: mode must be 0555')
        require(digest(path) == entry['sha256'], f'provider {entry["provider"]}: sha256 mismatch at {path}')
    except (OSError, RuntimeError) as exc:
        raise AllowlistError(f'provider {entry["provider"]}: missing or unreadable binary {path}: {exc}') from exc


def verify(document: dict[str, Any], bindings: list[dict[str, str]]) -> None:
    """Verify every active entry and each required stable launch entrypoint."""
    validate(document)
    active = {entry['provider']: entry for entry in document['entries'] if entry['active']}
    for entry in active.values():
        verify_entry(entry)
    seen: set[str] = set()
    paths: set[Path] = set()
    for binding in bindings:
        require(isinstance(binding, dict) and set(binding) == {'provider', 'path'}, 'invalid provider binding')
        provider = identifier(binding['provider'], 'binding provider')
        require(provider not in seen, f'duplicate binding for {provider}')
        seen.add(provider)
        require(provider in active, f'required active provider {provider} is missing')
        path = absolute(binding['path'], 'binding path')
        require(path not in paths, f'duplicate binding path {path}')
        paths.add(path)
        try:
            require(path.parent.resolve(strict=True) == path.parent,
                    f'provider {provider}: entrypoint parent is a symlink')
            require(path == Path(active[provider]['path']) or
                    (path.is_symlink() and os.readlink(path) == active[provider]['path']),
                    f'provider {provider}: entrypoint must link directly to active binary')
            require(str(path.resolve(strict=True)) == active[provider]['path'],
                    f'provider {provider}: entrypoint {path} does not resolve to active binary')
        except (OSError, RuntimeError) as exc:
            raise AllowlistError(f'provider {provider}: missing entrypoint {path}: {exc}') from exc


def closed_version(pin: dict[str, Any], sha: str) -> str | None:
    """Derive only a recognized closed wrapper's CLI component, retaining its source pin."""
    version = pin.get('version')
    match = CLOSED_VERSION.fullmatch(version) if isinstance(version, str) else None
    if match is None:
        return None
    require(isinstance(pin.get('path'), str) and Path(pin['path']).name == match[1]
            and pin.get('version_args') == ['--version'],
            'closed worker entrypoint/version arguments mismatch')
    require(bool(DIGEST.fullmatch(sha)), 'invalid closed worker CLI digest')
    return f'{match[1]} 2 sources_sha256={match[2]} cli_sha256={sha}'


def closed_selection(city: Path, executable: Path) -> tuple[bytes, dict[str, Any]]:
    """Discover verified Claude bytes; launch approval is a separate receipt check."""
    require(executable == city.parent / 'bin/claude', 'closed worker CLI binding is not canonical')
    path = city / 'managed/providers.json'
    require(path.resolve(strict=True) == path, 'closed worker authority path is a symlink')
    data = read_regular(path)
    document = parse(data)
    verify(document, [{'provider': 'claude', 'path': str(executable)}])
    entry = next(e for e in document['entries'] if e['provider'] == 'claude' and e['active'])
    return data, entry


def verify_closed_source(pin: dict[str, Any], city: Path | None = None) -> None:
    """Verify the physical source domain before executing any closed helper."""
    closed_version(pin, '0' * 64)
    path = absolute(pin['path'], 'closed worker path')
    require(path.resolve(strict=True) == path == Path(pin['resolved_path']),
            'closed worker requires its physical source path; symlink refused')
    require(path.parent.name == 'bin', 'closed worker requires a bin entrypoint')
    require(digest(path) == pin['sha256'], 'closed worker sha256 drift')
    match = CLOSED_VERSION.fullmatch(pin['version'])
    require(match is not None, 'invalid closed worker version')
    assert match is not None
    family = match[1].removeprefix('gct-claude-').removesuffix('-worker')
    root = path.parent.parent
    city = city or root.parent / 'gascity/city'
    label = 'core-signing' if family == 'signing' else family
    paths = [root / f'templates/claude/{label}-control-policy.json', path,
             root / f'lib/gct_claude_{family.replace("-", "_")}_worker.py']
    if family != 'signing':
        paths.append(root / 'lib/gct_claude_signing_worker.py')
    paths.append(root / 'lib/gct_claude_subscription.py')
    if family == 'project-candidate':
        paths.append(root / 'lib/gct_claude_workspace_trust.py')
    paths.extend([root / f'templates/claude/{family}-provider.toml',
                  root / 'lib/gct_provider_allowlist.py'])
    records = []
    for source in paths:
        require(source.resolve(strict=True) == source, 'closed worker source symlink refused')
        records.append({'path': str(source), 'sha256': digest(source)})
    domain = {'dependencies': records, 'provider': 'claude',
              'entrypoint': str(city.parent / 'bin/claude'),
              'allowlist': str(city / 'managed/providers.json')}
    encoded = json.dumps(domain, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()
    require(hashlib.sha256(encoded).hexdigest() == match[2], 'closed worker source version drift')


def launch_approval(city: Path, authority: bytes) -> bytes:
    """Require the protected receipt's self-digest and exact authority approval."""
    path = city / '.gc/runtime/provisioning/receipt.json'
    require(path.resolve(strict=True) == path, 'approval receipt path is a symlink')
    data = read_regular(path, 16 * MAX_BYTES)
    receipt = decode_json(data)
    require(validate_optional(receipt), 'approval receipt has no provider_allowlist pin')
    claimed = receipt.pop('receipt_sha256', None)
    encoded = json.dumps(receipt, ensure_ascii=False, sort_keys=True, separators=(',', ':'))
    for character in ('&', '<', '>', '\u2028', '\u2029'):
        encoded = encoded.replace(character, '\\u' + format(ord(character), '04x'))
    require(hashlib.sha256(encoded.encode()).hexdigest() == claimed,
            'approval receipt_sha256 mismatch')
    require(receipt.get('schema') in ('gc.provisioning-receipt.v1', 'gc.provisioning-receipt.v2'),
            'approval receipt schema is unsupported')
    revision = receipt.get('permission_revision')
    require(isinstance(revision, str) and bool(DIGEST.fullmatch(revision)),
            'approval receipt permission_revision is invalid')
    require(receipt['provider_allowlist'] == {
        'path': str(city / 'managed/providers.json'),
        'sha256': hashlib.sha256(authority).hexdigest(),
    }, 'provider allowlist approval path/sha256 mismatch; refresh receipt')
    return data


@contextmanager
def opened_executable(entry: dict[str, Any]) -> Iterator[int]:
    """Verify and retain the same executable descriptor for auth and final exec."""
    descriptor = os.open(entry['path'], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        info = os.fstat(descriptor)
        require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o555,
                'provider executable must be a regular 0555 file')
        with os.fdopen(os.dup(descriptor), 'rb') as stream:
            require(hashlib.file_digest(stream, 'sha256').hexdigest() == entry['sha256'],
                    'provider executable sha256 changed before execution')
        os.lseek(descriptor, 0, os.SEEK_SET)
        # Interpreters must also retain this descriptor while opening script bytes.
        os.set_inheritable(descriptor, True)
        yield descriptor
    finally:
        os.close(descriptor)


def closed_identity(family: str, paths: tuple[Path, ...], source_digests: dict[Path, str],
                    stable_digest: Any, city: Path, executable: Path) -> str:
    """Hash all non-CLI dependencies and the fixed authority binding independently."""
    _, entry = closed_selection(city, executable)
    require(len(set(paths)) == len(paths), 'dependency paths must be unique')
    records = [{'path': str(path), 'sha256': source_digests.get(path)
                or stable_digest(path, f'dependency[{index}]')} for index, path in enumerate(paths)]
    domain = {'dependencies': records, 'provider': 'claude', 'entrypoint': str(executable),
              'allowlist': str(city / 'managed/providers.json')}
    sha = hashlib.sha256(json.dumps(domain, ensure_ascii=False, sort_keys=True,
                                    separators=(',', ':')).encode()).hexdigest()
    return f'{family} 2 sources_sha256={sha} cli_sha256={entry["sha256"]}'
