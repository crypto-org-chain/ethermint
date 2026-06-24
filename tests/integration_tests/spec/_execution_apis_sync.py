import os
import shutil
import sys
import tempfile
import urllib.request
import zipfile
from pathlib import Path

SPEC_ROOT = Path(__file__).parent
EXECUTION_APIS_FIXTURE_DIR = SPEC_ROOT / "fixtures" / "execution_apis"

DEFAULT_EXECUTION_APIS_REF = "main"
EXECUTION_APIS_ARCHIVE_URL = (
    "https://github.com/ethereum/execution-apis/archive/{ref}.zip"
)

TEST_FIXTURE_FILES = [
    "chain.rlp",
    "forkenv.json",
    "genesis.json",
    "headfcu.json",
]
TOOL_CHAIN_FIXTURE_FILES = [
    "accounts.json",
    "headstate.json",
    "txinfo.json",
]


def _env_disabled(value):
    return str(value).lower() in {"0", "false", "no", "off"}


def _download_archive(ref, archive_path):
    archive_url = os.environ.get("EXECUTION_APIS_ARCHIVE_URL")
    if archive_url is None:
        archive_url = EXECUTION_APIS_ARCHIVE_URL.format(ref=ref)

    request = urllib.request.Request(
        archive_url,
        headers={"User-Agent": "ethermint-rpc-schema-tests"},
    )
    with urllib.request.urlopen(request, timeout=120) as response:
        archive_path.write_bytes(response.read())
    return archive_url


def _extract_archive(archive_path, extract_dir):
    with zipfile.ZipFile(archive_path) as archive:
        for member in archive.infolist():
            path = Path(member.filename)
            if path.is_absolute() or ".." in path.parts:
                raise ValueError(f"unsafe archive path: {member.filename}")
        archive.extractall(extract_dir)

    roots = [path for path in extract_dir.iterdir() if path.is_dir()]
    if len(roots) != 1:
        raise ValueError(f"expected one execution-apis archive root, got {roots}")
    return roots[0]


def _remove_copied_io_fixtures():
    for child in SPEC_ROOT.iterdir():
        if child.is_dir() and any(child.glob("*.io")):
            shutil.rmtree(child)


def _copy_io_fixtures(source_tests_dir):
    _remove_copied_io_fixtures()
    copied = 0
    for source in sorted(source_tests_dir.glob("*/*.io")):
        target = SPEC_ROOT / source.relative_to(source_tests_dir)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        copied += 1
    if copied == 0:
        raise ValueError(f"no execution-apis .io fixtures found in {source_tests_dir}")
    return copied


def _copy_required_files(source_dir, target_dir, names):
    for name in names:
        source = source_dir / name
        if not source.exists():
            raise FileNotFoundError(f"execution-apis fixture missing: {source}")
        target_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target_dir / name)


def _regenerate_ethermint_genesis_overlay():
    sys.path.insert(0, str(EXECUTION_APIS_FIXTURE_DIR))
    try:
        from generate_ethermint_genesis import main as generate_overlay

        generate_overlay()
    finally:
        sys.path.remove(str(EXECUTION_APIS_FIXTURE_DIR))


def sync_execution_apis():
    if _env_disabled(os.environ.get("EXECUTION_APIS_SYNC", "1")):
        return "execution-apis sync disabled by EXECUTION_APIS_SYNC"

    ref = os.environ.get("EXECUTION_APIS_REF", DEFAULT_EXECUTION_APIS_REF)
    with tempfile.TemporaryDirectory(prefix="execution-apis-") as tmp:
        tmp_path = Path(tmp)
        archive_path = tmp_path / "execution-apis.zip"
        archive_url = _download_archive(ref, archive_path)
        upstream_root = _extract_archive(archive_path, tmp_path / "src")

        source_tests_dir = upstream_root / "tests"
        source_tools_chain_dir = upstream_root / "tools" / "chain"
        copied = _copy_io_fixtures(source_tests_dir)
        _copy_required_files(
            source_tests_dir,
            EXECUTION_APIS_FIXTURE_DIR,
            TEST_FIXTURE_FILES,
        )
        _copy_required_files(
            source_tools_chain_dir,
            EXECUTION_APIS_FIXTURE_DIR,
            TOOL_CHAIN_FIXTURE_FILES,
        )
        _regenerate_ethermint_genesis_overlay()

    return f"synced {copied} execution-apis fixtures from {archive_url}"
