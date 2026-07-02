local config = import 'simulate.jsonnet';

config {
  'ethermint_9000-1'+: {
    'app-config'+: {
      'json-rpc'+: {
        api: 'eth,net,web3,debug,txpool',
        'feehistory-cap': 100,
        'block-range-cap': 10000,
        'logs-cap': 10000,
        'return-data-limit': 300000,
      },
    },
    genesis+: {
      app_state+: {
        feemarket+: {
          params+: {
            no_base_fee: false,
            base_fee: '100000000000',
            min_gas_multiplier: '0',
          },
        },
      },
    },
  },
}
