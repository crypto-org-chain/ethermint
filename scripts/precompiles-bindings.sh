#!/bin/sh

pushd precompiles/bindings

imageName=ethereum/solc:stable
solc="docker run --rm -v $(pwd):/workspace --workdir /workspace ${imageName}"

${solc} --abi --bin src/CosmosTypes.sol -o build --overwrite
${solc} --abi --bin src/Relayer.sol -o build --overwrite
${solc} --abi --bin src/RelayerFunctions.sol -o build --overwrite
${solc} --abi --bin src/Bank.sol -o build --overwrite
${solc} --abi --bin src/ICA.sol -o build --overwrite
${solc} --abi --bin src/ICACallback.sol -o build --overwrite


abigen="go run github.com/ethereum/go-ethereum/cmd/abigen@latest"
mkdir -p cosmos/lib && \
${abigen} --pkg lib --abi build/CosmosTypes.abi --bin build/CosmosTypes.bin --out cosmos/lib/cosmos_types.abigen.go --type CosmosTypes

mkdir -p cosmos/precompile/relayer && \
${abigen} --pkg relayer --abi build/IRelayerModule.abi --bin build/IRelayerModule.bin --out cosmos/precompile/relayer/i_relayer_module.abigen.go --type RelayerModule
${abigen} --pkg relayer --abi build/IRelayerFunctions.abi --bin build/IRelayerFunctions.bin --out cosmos/precompile/relayer/i_relayer_functions.abigen.go --type RelayerFunctions

mkdir -p cosmos/precompile/bank && \
${abigen} --pkg bank --abi build/IBankModule.abi --bin build/IBankModule.bin --out cosmos/precompile/bank/i_bank_module.abigen.go --type BankModule

mkdir -p cosmos/precompile/ica && \
${abigen} --pkg ica --abi build/IICAModule.abi --bin build/IICAModule.bin --out cosmos/precompile/ica/i_ica_module.abigen.go --type ICAModule
mkdir -p cosmos/precompile/icacallback && \
${abigen} --pkg icacallback --abi build/IICACallback.abi --bin build/IICACallback.bin --out cosmos/precompile/icacallback/i_ica_callback.abigen.go --type ICACallback

popd