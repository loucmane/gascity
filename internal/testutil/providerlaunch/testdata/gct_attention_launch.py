"""Launch native or attention providers through approved, retained executable descriptors."""

from __future__ import annotations

import os
from pathlib import Path
import runpy
import sys
from typing import Callable

POLICY = runpy.run_path(str(Path(__file__).with_name('gct_provider_allowlist.py')))
require = POLICY['require']


def claude_arguments(city: Path, arguments: list[str], environment: dict[str, str]) -> list[str]:
    """Replace exactly one generated settings argument with the closed tool surface."""
    shared = environment.get('GC_ATTENTION_SHARED_SETTINGS') or str(city / '.gc/settings.json')
    closed = environment.get('GC_ATTENTION_SETTINGS') or str(city / 'hooks/claude-attention.json')
    require(os.access(closed, os.R_OK), f'closed settings are not readable: {closed}')
    result: list[str] = []
    seen = 0
    index = 0
    refused = {'--tools', '--allowedTools', '--allowed-tools', '--disallowedTools',
               '--disallowed-tools', '--dangerously-skip-permissions',
               '--allow-dangerously-skip-permissions'}
    while index < len(arguments):
        argument = arguments[index]
        if argument == '--settings':
            require(index + 1 < len(arguments), '--settings is missing its value')
            require(arguments[index + 1] == shared,
                    f'unexpected --settings source: {arguments[index + 1]}')
            seen += 1
            index += 2
        else:
            require(argument not in refused, f'refusing caller-owned tool or bypass flag: {argument}')
            result.append(argument)
            index += 1
    require(seen == 1, f'expected exactly one generated --settings source, got {seen}')
    return [*result, '--safe-mode', '--disable-slash-commands', '--tools', 'Bash', '--settings', closed]


def check_config(value: str) -> None:
    """Refuse every supported spelling of provider-owned startup settings."""
    key = value.removeprefix('=').split('=', 1)[0]
    key = ''.join(key.split()).replace('"', '').replace("'", '')
    require(key != 'features' and not key.startswith('features.')
            and key != 'check_for_update_on_startup', 'refusing caller-owned startup config')


def codex_arguments(arguments: list[str]) -> list[str]:
    """Keep feature, update and bypass policy under the wrapper's authority."""
    index = 0
    while index < len(arguments):
        argument = arguments[index]
        require(argument not in {'--enable', '--disable'}
                and not argument.startswith(('--enable=', '--disable=')),
                f'refusing caller-owned feature flag: {argument}')
        require(argument != '--profile' and not argument.startswith(('--profile=', '-p'))
                and argument not in {'--dangerously-bypass-hook-trust',
                                     '--dangerously-bypass-approvals-and-sandbox', '--approve-for-me'},
                f'refusing caller-owned profile or bypass flag: {argument}')
        if argument in ('-c', '--config'):
            require(index + 1 < len(arguments), f'{argument} is missing its value')
            index += 1
            check_config(arguments[index])
        elif argument.startswith('--config='):
            check_config(argument.removeprefix('--config='))
        elif argument.startswith('-c'):
            check_config(argument[2:])
        index += 1
    return ['--disable', 'hooks', '-c', 'check_for_update_on_startup=false', *arguments]


def launch(provider: str, city: Path, arguments: list[str], environment: dict[str, str], *,
           execve: Callable[[int | str, list[str], dict[str, str]], object] = os.execve) -> None:
    """Verify the direct binding and execute the same opened bytes across path replacement."""
    native = provider.startswith('native:')
    provider = provider.removeprefix('native:')
    require(provider in {'claude', 'codex'}, 'unsupported attention provider')
    require(native or f'GC_ATTENTION_{provider.upper()}_BIN' not in environment,
            'refusing caller-owned binary override')
    authority = city / 'managed/providers.json'
    require(authority.resolve(strict=True) == authority, 'authority path is a symlink')
    data = POLICY['read_regular'](authority)
    document = POLICY['parse'](data)
    approval = POLICY['launch_approval'](city, data)
    bindings = [{'provider': provider, 'path': str(city.parent / 'bin' / provider)}]
    POLICY['verify'](document, bindings)
    entry = next(item for item in document['entries'] if item['provider'] == provider and item['active'])
    executable = entry['path']
    if not native and provider == 'claude':
        arguments = claude_arguments(city, arguments, environment)
        environment = {**environment, 'DISABLE_AUTOUPDATER': '1'}
    elif not native:
        arguments = codex_arguments(arguments)
    with POLICY['opened_executable'](entry) as descriptor:
        require(POLICY['read_regular'](authority) == data, 'provider authority changed before execution')
        POLICY['verify'](document, bindings)
        require(POLICY['launch_approval'](city, data) == approval,
                'provider approval receipt changed before execution')
        # Native Core commands preserve their CLI policy and arguments. The
        # descriptor stays inheritable for script interpreters as well as ELF.
        target = f'/proc/self/fd/{descriptor}' if native else descriptor
        execve(target, [executable, *arguments], environment)


def main() -> int:
    """Report a contextual refusal without launching any unapproved provider."""
    try:
        require(len(sys.argv) >= 3, 'expected provider and city')
        launch(sys.argv[1], Path(sys.argv[2]), sys.argv[3:], dict(os.environ))
    except (OSError, RuntimeError, ValueError) as exc:
        print(f'{sys.argv[1] if len(sys.argv) > 1 else "provider"}-attention: {exc}', file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
