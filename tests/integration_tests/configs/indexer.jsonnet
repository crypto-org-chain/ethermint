local config = import 'default.jsonnet';

config {
  'ethermint-9000'+: {
    'app-config'+: {
      pruning: 'custom',
      'pruning-keep-recent': 2,
      'pruning-interval': 10,
      'min-retain-blocks': 2,
      'state-sync'+: {
        'snapshot-interval': 1,
      },
      'json-rpc'+: {
        'enable-indexer': true,
      },
    },
    genesis+: {
      consensus+: {
        params+: {
          evidence+: {
            max_age_num_blocks: '10',
          },
        },
      },
    },
  },
}
