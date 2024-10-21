{
  dotenv: '../../../scripts/env',
  'ethermint_9000-1': {
    cmd: 'ethermintd',
    'start-flags': '--trace',
    'app-config': {
      'minimum-gas-prices': '0aphoton',
      'index-events': ['ethereum_tx.ethereumTxHash'],
      'json-rpc': {
        address: '127.0.0.1:{EVMRPC_PORT}',
        'ws-address': '127.0.0.1:{EVMRPC_PORT_WS}',
        api: 'eth,net,web3,debug',
        'feehistory-cap': 100,
        'block-range-cap': 10000,
        'logs-cap': 10000,
        'fix-revert-gas-refund-height': 1,
      },
    },
    validators: [{
      coins: '1000000000000000000stake,10000000000000000000000aphoton',
      staked: '1000000000000000000stake',
      mnemonic: '${VALIDATOR1_MNEMONIC}',
      client_config: {
        'broadcast-mode': 'sync',
      },
      'app-config': {
        evm: {
          'block-executor': 'block-stm',
          'block-stm-workers': 32,
        },
      },
    }, {
      coins: '1000000000000000000stake,10000000000000000000000aphoton',
      staked: '1000000000000000000stake',
      mnemonic: '${VALIDATOR2_MNEMONIC}',
      client_config: {
        'broadcast-mode': 'sync',
      },
    }],
    accounts: [{
      name: 'community',
      coins: '10000000000000000000000aphoton',
      mnemonic: '${COMMUNITY_MNEMONIC}',
    }, {
      name: 'signer1',
      coins: '20000000000000000000000aphoton',
      mnemonic: '${SIGNER1_MNEMONIC}',
    }, {
      name: 'signer2',
      coins: '30000000000000000000000aphoton',
      mnemonic: '${SIGNER2_MNEMONIC}',
    }],
    genesis: {
      consensus: {
        params: {
          block: {
            max_bytes: '1048576',
            max_gas: '81500000',
          },
        },
      },
      app_state: {
        evm: {
          params: {
            evm_denom: 'aphoton',
          },
        },
        gov: {
          params: {
            expedited_voting_period: '1s',
            voting_period: '10s',
            max_deposit_period: '10s',
            min_deposit: [
              {
                denom: 'aphoton',
                amount: '1',
              },
            ],
          },
        },
        transfer: {
          params: {
            receive_enabled: true,
            send_enabled: true,
          },
        },
        feemarket: {
          params: {
            no_base_fee: false,
            base_fee: '100000000000',
            min_gas_multiplier: '0.5',
          },
        },
      },
    },
  },
}
