#!/bin/sh
set -e
cd "$(dirname "$0")"

# explicitly set a short TMPDIR to prevent path too long issue on macosx
export TMPDIR=/tmp

echo "build test contracts"
cd ../tests/integration_tests/hardhat
HUSKY_SKIP_INSTALL=1 npm install
npm run typechain
cd ..

TESTS_TO_RUN="${TESTS_TO_RUN:-all}"

for i in $(seq 1 10);
do
    echo "run tests: $i"
    pytest -v -s --timeout=60 test_priority.py::test_native_tx_priority
done