package provider

#EnvironmentName: string & =~"^[A-Za-z_][A-Za-z0-9_]*$"
#ProviderName: string & =~"^[a-z][a-z0-9._-]*$"
#Duration: string & =~"^[0-9]+(ns|us|µs|ms|s|m|h)$"

#Action: {
	description: string & !=""
	argv?: [...string]
	env?: [#EnvironmentName]: string
}

#Provider: {
	version: "provider/v1"
	name: #ProviderName
	description: string & !=""
	command: [string & !="", ...string]
	actions: [#ProviderName]: #Action
	requires?: {
		commands?: [...string & !=""]
		environment?: [...#EnvironmentName]
		paths?: [...string & !=""]
	}
	defaults?: {
		timeout?: #Duration
		priority?: int
	}
}
