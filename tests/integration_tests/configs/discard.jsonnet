local config = import 'default.jsonnet';

config {
  'ethermint-9000'+: {
    config+: {
      storage+: {
        discard_abci_responses: true,
      },
    },
  },
}
