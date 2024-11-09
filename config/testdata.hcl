default {
	model = "4o"
}

auth "keyring" "default" {
	# use macOS system keychain
	backend = "keychain"
}

# provider "driver" "[identifier]"
provider "openai" "llama-cpp-python" {
	base_url = "http://localhost:8000/v1"
	model "gemma-2-9b-simpo" {
		alias = ["g9s"]
		tags = ["q4_0", "q8_0"] # the first tag is the default
		context_length = 8192
		max_tokens = 4096
	}
	model "gemma-2-27b-it" {
		alias = ["g27"]
		tags = ["q4_0"]
		context_length = 8192
		max_tokens = 4096
	}
}
provider "anthropic" "api" {
	model "claude-3-5-sonnet" {
		alias = ["cs35"]
		latest = true
	}
	model "claude-3-5-haiku" {
		alias = ["ch35"]
		latest = true
	}
}

# simp will use the daemon by default
daemon {
	listen_addr = "0.0.0.0:51015"
	allowed_ips = ["10.0.0.0/8"]
}

history {
	# group histories from all projects by project name
	path "~/projects/*/**" {
		group = "*"
	}
	# appropriately annotate histories, i.e. a discussion on HCL parsing
	# might be saved as parsing-hcl-configs.md as opposed to a timestamp
	# that would be assigned otherwise
	#
	# NOTE: annotations require a running daemon!
	annotate = true
	annotate_with = "cs35"
}