local config = import 'default.jsonnet';

config {
  'ethermint-9000'+: {
    'app-config'+: {
      pruning: 'everything',
      'state-sync'+: {
        'snapshot-interval': 0,
      },
    },
    genesis+: {
      app_state+: {
        feemarket+: {
          params+: {
            min_gas_multiplier: '0',
          },
        },
      },
    },
  },
}
