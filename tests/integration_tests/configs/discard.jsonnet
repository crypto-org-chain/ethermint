local config = import 'default.jsonnet';

config {
  'ethermint_9000-1'+: {
    config+: {
      storage+: {
        discard_abci_responses: true,
      },
    },
  },
}
