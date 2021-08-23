import { registry, injectable } from "tsyringe";
import * as ChildProcess from "child_process"
import { SpawnSyncReturns } from "child_process"
import { ShellCommand } from "./devEnv"
import { GolangResultsPromise } from "./golangBuilder";
import * as path from "path"
import events from "events";
import { lastValueFrom, ReplaySubject } from "rxjs";
import { ValidatorValues } from "./sifnoded"
import * as fs from "fs";
import YAML from 'yaml'
import { eventEmitterToObservable } from "./devEnvUtilities"

@registry([
  {
    token: EbrelayerArguments, useValue: new EbrelayerArguments(
      "/tmp/sifnoded.log",
      9000,
      1,
      "localnet",
      "/tmp/sifnodedConfig.yml",
      "/tmp/sifnodedNetwork",
      "10.10.1.1",
      "../test/integration/whitelisted-denoms.json"
    )
  }
])
export class EbrelayerArguments {
  constructor(
    readonly websocketAddress: string,
    // Interface in hardhatNode readonly bridgeRegistryAddress: string,
    readonly tcpURL: string,
    readonly chainNet: number,
    readonly ebrelayerDB: string,
    readonly validatorValues: ValidatorValues,
    readonly symbolTranslatorFile: string,
    readonly relayerdbPath: string,
  ) {
  }
}

interface EbrelayerResults {
}

@injectable()
export class EbrelayerRunner extends ShellCommand<EbrelayerResults> {
  constructor(
    readonly args: EbrelayerArguments,
    readonly golangResults: GolangResultsPromise
  ) {
    super();
  }

  cmd(): [string, string[]] {
    return ["ebrelayer", [
      "init",
      this.args.tcpURL,
      this.args.websocketAddress,
      // Bridge Registery Address,
      this.args.validatorValues.moniker,
      this.args.validatorValues.mnemonic,
      `--chain-id ${this.args.chainNet}`,
      `--node ${this.args.tcpURL}`,
      "--keyring-backend test",
      `--from ${this.args.validatorValues.moniker}`,
      `--symbol-translator-file ${this.args.symbolTranslatorFile}`,
      `--relayerdb-path ${this.args.relayerdbPath}`
    ]]
  }

  async sifgenNetworkCreate() {
    const sifnodedCommand = path.join((await this.golangResults.results).goBin, "sifnoded")
    const sifgenArgs = [
      "network",
      "create",
      "--keyring-backend",
      "test",
      this.args.chainId,
      this.args.nValidators.toString(),
      this.args.networkDir,
      this.args.seedIpAddress,
      this.args.networkConfigFile
    ]
    const sifgenOutput = ChildProcess.execFileSync(
      path.join((await this.golangResults.results).goBin, "sifgen"),
      sifgenArgs,
      { encoding: "utf8" }
    )
    const file = fs.readFileSync(this.args.networkConfigFile, 'utf8')
    const networkConfig = YAML.parse(file)
    console.log("ymlis: ", JSON.stringify(networkConfig, undefined, 2))
    const moniker = networkConfig[0]["moniker"]
    let mnemonic = networkConfig[0]["mnemonic"]
    let password = networkConfig[0]["password"]
    const chainDir = path.join(
      this.args.networkDir,
      "validators",
      this.args.chainId,
      moniker
    )
    const homeDir = path.join(chainDir, ".sifnoded")
    await this.addValidatorKeyToTestKeyring(
      moniker,
      this.args.networkDir,
      mnemonic,
    )
    const valOperKey = await this.readValoperKey(
      moniker,
      this.args.networkDir,
      mnemonic,
    )
    await this.addGenesisValidator(chainDir, valOperKey)
    const whitelistedValidator = ChildProcess.execSync(
      `${sifnodedCommand} keys show -a --bech val ${moniker} --keyring-backend test`,
      { encoding: "utf8", input: password }
    ).trim()
    let sifnodeadmincmd = `${sifnodedCommand} keys add sifnodeadmin --keyring-backend test --output json`;
    const sifnodedadminJson = ChildProcess.execSync(
      sifnodeadmincmd,
      { encoding: "utf8", input: "yes\nyes" }
    ).trim()
    const sifnodedAdminAddress = JSON.parse(sifnodedadminJson)["address"]
    // const q = ChildProcess.execSync(
    //     `${sifnodedCommand} add-genesis-validators ${whitelistedValidator} --home ${homeDir}`,
    //     {encoding: "utf8", input: password}
    // ).trim()
    // sifnoded add-genesis-account $adminuser 100000000000000000000rowan --home $CHAINDIR/.sifnoded
    // sifnoded set-genesis-oracle-admin $adminuser --home $CHAINDIR/.sifnoded
    // sifnoded set-genesis-whitelister-admin $adminuser --home $CHAINDIR/.sifnoded
    // sifnoded set-gen-denom-whitelist $SCRIPT_DIR/whitelisted-denoms.json --home $CHAINDIR/.sifnoded
    ChildProcess.execSync(
      `${sifnodedCommand} add-genesis-account ${sifnodedAdminAddress} 100000000000000000000rowan --home ${homeDir}`,
      { encoding: "utf8" }
    ).trim()
    ChildProcess.execSync(
      `${sifnodedCommand} set-genesis-oracle-admin ${sifnodedAdminAddress} --home ${homeDir}`,
      { encoding: "utf8" }
    ).trim()
    ChildProcess.execSync(
      `${sifnodedCommand} set-genesis-whitelister-admin ${sifnodedAdminAddress} --home ${homeDir}`,
      { encoding: "utf8" }
    ).trim()
    ChildProcess.execSync(
      `${sifnodedCommand} set-gen-denom-whitelist ${this.args.whitelistFile} --home ${homeDir}`,
      { encoding: "utf8" }
    ).trim()
    let sifnodedDaemonCmd = `${sifnodedCommand} start --minimum-gas-prices 0.5rowan --rpc.laddr tcp://0.0.0.0:26657 --home ${homeDir}`;
    const sifnoded = ChildProcess.spawn(
      sifnodedDaemonCmd,
      { shell: true, stdio: "inherit" }
    )
    return lastValueFrom(eventEmitterToObservable(sifnoded, "sifnoded"))
  }

  async addValidatorKeyToTestKeyring(moniker: string, chainDir: string, mnemonic: string) {
    const sifgenArgs = [
      "keys",
      "add",
      moniker,
      "--keyring-backend",
      "test",
    ]
    let child = ChildProcess.execFileSync(
      path.join((await this.golangResults.results).goBin, "sifnoded"),
      sifgenArgs,
      { input: mnemonic, encoding: "utf8" }
    );
    child
  }

  async readValoperKey(moniker: string, chainDir: string, mnemonic: string): Promise<string> {
    const sifgenArgs = [
      "keys",
      "show",
      "-a",
      "--bech",
      "val",
      moniker,
      // "--home",
      // path.join(chainDir, ".sifnoded"),
      "--keyring-backend",
      "test",
    ]
    return ChildProcess.execFileSync(
      path.join((await this.golangResults.results).goBin, "sifnoded"),
      sifgenArgs,
      { encoding: "utf8" }
    ).trim()
  }

  // sifnoded add-genesis-validators $valoper --home $CHAINDIR/.sifnoded
  async addGenesisValidator(chainDir: string, valoper: string): Promise<string> {
    const sifgenArgs = [
      "add-genesis-validators",
      valoper,
      "--home",
      path.join(chainDir, ".sifnoded"),
    ]
    return ChildProcess.execFileSync(
      path.join((await this.golangResults.results).goBin, "sifnoded"),
      sifgenArgs,
      { encoding: "utf8" }
    )
  }

  async execute() {
    await this.sifgenNetworkCreate()
    console.log("finisehd sifnodedts execute")
  }

  override run(): Promise<void> {
    console.log("inrun")
    return this.execute()
  }

  override async results(): Promise<SifnodedResults> {
    return Promise.resolve({})
  }
}
