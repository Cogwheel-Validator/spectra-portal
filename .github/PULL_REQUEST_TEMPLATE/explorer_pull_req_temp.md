# New Explorer Submission

This submission is for any new explorer that is not on the list of known explorers.
Even if this is just a fork of an existing explorer, for example PingPub, you should submit it here.
To check if your explorer is already on the list, you can check the `explorers/README.md` file.

The reason for this kind of PR is to allow the client app to use this explorer. This PR does not guarantee that the explorer will be used. Due to the project being open source, anyone can submit a PR to add their explorer to the list. And the priority explorer **will always be Spectra** due to the project being tied to it. If the Spectra doesn't support some specific chain, another will be used from the list of the explorers if they support it.

## Explorer Information

If you are submitting a fork, please provide the following information:

- Fork Name: The name of the fork.
- Fork Github Repository: The github repository of the fork.

If this is not the case, or you are the OG developer of the explorer, skip the Fork Name and Fork Github Repository fields.

- **Name:** ( e.g "Spectra", "Mintscan", "PingPub")
- **Base URL:** ( e.g "<https://thespectra.io>", "<https://mintscan.io>", "<https://ping.pub>")
- **Mult Chain Support:** (e.g. if "Yes", then the transaction and account paths will be different for each chain)
- **Transaction Path:** (e.g. "<https://thespectra.io/chain_name/transactions/tx_hash>" or "<https://explorer.com/tx/tx_hash>")
- **Account Path:** (e.g. "<https://thespectra.io/chain_name/account/address>" or "<https://explorer.com/account/address>")
- **Is Fork:** (e.g. "Yes", "No")
- **Fork Name:** (e.g "PingPub")
- **Fork Github Repository:** (e.g "<https://github.com/ping-pub/explorer>")

## Your Information

If you are a Blockchain Validator, or developers behind the blockchain that have unique explorer for their blockchain you will need to fill out some additional information:

- **Validator or Chain Name:** (e.g "Cogwheel Validator", "Berachain" etc...) ( applicable only for the validator or chain developers)
- **Website URL:** (e.g "<https://cogwheel.zone>") ( applicable only for the validator or chain developers)
- **Contact Email:** ( applicable only for the validator or chain developers)
- **Github Profile:** ( both organization and personal profiles are acceptable)

If you are not any of the above mentioned, skip all of it. If you are the developer behind a explorer, yet you do not fall into the categories above, leave only your github link to the profile. If you are neither we still require some contact of the owner of the explorer. So if you can find out his/her github profile, please provide it. Even if the explorer is closed source, we still require some contact of the owner of the explorer.

### Verification Checklist

- [ ] I have tested the explorer with real transactions
- [ ] The explorer is actively maintained
- [ ] The explorer uses HTTPS
- [ ] This is a legitimate blockchain explorer (not a phishing site)

### Example URLs

Provide working example URLs:

- TX:
- Account:

## Additional Context

If you feel like you need to provide some additional context, you can do so here.
