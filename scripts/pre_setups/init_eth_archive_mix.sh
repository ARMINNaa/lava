#!/bin/bash 
__dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "$__dir"/../useful_commands.sh
. "${__dir}"/../vars/variables.sh

LOGS_DIR=${__dir}/../../testutil/debugging/logs
mkdir -p $LOGS_DIR
rm $LOGS_DIR/*.log

killall screen
screen -wipe

GASPRICE="0.000000001ulava"
if [ $# -eq 0 ]; then
    lavad tx gov submit-legacy-proposal spec-add ./cookbook/specs/spec_add_ibc.json,./cookbook/specs/spec_add_cosmoswasm.json,./cookbook/specs/spec_add_cosmossdk.json,./cookbook/specs/spec_add_cosmossdk_45.json,./cookbook/specs/spec_add_cosmossdk_full.json,./cookbook/specs/spec_add_ethereum.json,./cookbook/specs/spec_add_cosmoshub.json,./cookbook/specs/spec_add_lava.json,./cookbook/specs/spec_add_osmosis.json,./cookbook/specs/spec_add_fantom.json,./cookbook/specs/spec_add_celo.json,./cookbook/specs/spec_add_optimism.json,./cookbook/specs/spec_add_arbitrum.json,./cookbook/specs/spec_add_starknet.json,./cookbook/specs/spec_add_aptos.json,./cookbook/specs/spec_add_juno.json,./cookbook/specs/spec_add_polygon.json,./cookbook/specs/spec_add_evmos.json,./cookbook/specs/spec_add_base.json,./cookbook/specs/spec_add_canto.json,./cookbook/specs/spec_add_sui.json,./cookbook/specs/spec_add_solana.json,./cookbook/specs/spec_add_bsc.json,./cookbook/specs/spec_add_axelar.json,./cookbook/specs/spec_add_avalanche.json,./cookbook/specs/spec_add_fvm.json --lava-dev-test -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE &
    wait_next_block
    wait_next_block
    lavad tx gov vote 1 yes -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
    sleep 4

    # Plans proposal
    lavad tx gov submit-legacy-proposal plans-add ./cookbook/plans/test_plans/default.json,./cookbook/plans/test_plans/temporary-add.json -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
    wait_next_block
    wait_next_block
    lavad tx gov vote 2 yes -y --from alice --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE

    sleep 4

    CLIENTSTAKE="500000000000ulava"
    PROVIDERSTAKE="500000000000ulava"

    PROVIDER1_LISTENER="127.0.0.1:2221"
    PROVIDER2_LISTENER="127.0.0.1:2222"
    PROVIDER3_LISTENER="127.0.0.1:2223"
    PROVIDER4_LISTENER="127.0.0.1:2224"
    PROVIDER5_LISTENER="127.0.0.1:2225"

    lavad tx subscription buy DefaultPlan $(lavad keys show user1 -a) -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE
    wait_next_block

    lavad tx project set-policy $(lavad keys show user1 -a)-admin ./cookbook/projects/policy_all_chains_with_extension.yml -y --from user1 --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE

    lavad tx pairing stake-provider "ETH1" $PROVIDERSTAKE "$PROVIDER1_LISTENER,1" 1 $(operator_address) -y --from servicer1 --provider-moniker "servicer1" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE --delegate-commission 50 --delegate-limit $PROVIDERSTAKE
    lavad tx pairing stake-provider "ETH1" $PROVIDERSTAKE "$PROVIDER2_LISTENER,2" 2 $(operator_address) -y --from servicer2 --provider-moniker "servicer2" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE --delegate-commission 50 --delegate-limit $PROVIDERSTAKE
    lavad tx pairing stake-provider "ETH1" $PROVIDERSTAKE "$PROVIDER3_LISTENER,2" 2 $(operator_address) -y --from servicer3 --provider-moniker "servicer3" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE --delegate-commission 50 --delegate-limit $PROVIDERSTAKE
    lavad tx pairing stake-provider "ETH1" $PROVIDERSTAKE "$PROVIDER4_LISTENER,2,archive" 2 $(operator_address) -y --from servicer4 --provider-moniker "servicer4" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE --delegate-commission 50 --delegate-limit $PROVIDERSTAKE
    lavad tx pairing stake-provider "ETH1" $PROVIDERSTAKE "$PROVIDER5_LISTENER,1,archive" 1 $(operator_address) -y --from servicer5 --provider-moniker "servicer5" --gas-adjustment "1.5" --gas "auto" --gas-prices $GASPRICE --delegate-commission 50 --delegate-limit $PROVIDERSTAKE

    sleep_until_next_epoch
    lavad q pairing effective-policy ETH1 --from user1
fi


screen -d -m -S provider1 bash -c "source ~/.bashrc; lavap rpcprovider \
$PROVIDER1_LISTENER ETH1 jsonrpc '$ETH_RPC_WS' \
$EXTRA_PROVIDER_FLAGS --geolocation 1 --log_level debug --from servicer1 --chain-id lava 2>&1 | tee $LOGS_DIR/PROVIDER1.log" && sleep 0.25

i=1
screen -d -m -S provider$i bash -c "source ~/.bashrc; lavap rpcprovider \
127.0.0.1:222$i ETH1 jsonrpc '$ETH_RPC_WS' \
$EXTRA_PROVIDER_FLAGS --geolocation 1 --log_level debug --from servicer$i --chain-id lava 2>&1 | tee $LOGS_DIR/PROVIDER$i.log" && sleep 0.25

for i in {2..4}; do
  screen -d -m -S provider$i bash -c "source ~/.bashrc; lavap rpcprovider \
  127.0.0.1:222$i ETH1 jsonrpc '$ETH_RPC_WS' \
  $EXTRA_PROVIDER_FLAGS --geolocation 2 --log_level debug --from servicer$i --chain-id lava 2>&1 | tee $LOGS_DIR/PROVIDER$i.log" && sleep 0.25
done

i=5
screen -d -m -S provider$i bash -c "source ~/.bashrc; lavap rpcprovider \
127.0.0.1:222$i ETH1 jsonrpc '$ETH_RPC_WS' \
$EXTRA_PROVIDER_FLAGS --geolocation 1 --log_level debug --from servicer$i --chain-id lava 2>&1 | tee $LOGS_DIR/PROVIDER$i.log" && sleep 0.25

screen -d -m -S portals bash -c "source ~/.bashrc; lavap rpcconsumer consumer_examples/ethereum_example.yml\
$EXTRA_PORTAL_FLAGS --cache-be "127.0.0.1:7778" --geolocation 1 --debug-relays --log_level debug --from user1 --chain-id lava --allow-insecure-provider-dialing 2>&1 | tee $LOGS_DIR/PORTAL.log" && sleep 0.25
echo "--- setting up screens done ---"
screen -ls

sleep 2
lavad test rpcconsumer http://127.0.0.1:3333 ETH1 jsonrpc --chain-id lava
