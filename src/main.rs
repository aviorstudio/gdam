use gdam::{
    Result, commands, install, manifest::Manifest, network::Network, project, transaction::Project,
};
use std::{path::PathBuf, time::Duration};
fn usage() -> String {
    String::from(
        "gdam - Godot addon manager (GitHub addons installer)\n\nUsage:\n  gdam --version\n  gdam init\n  gdam add @username/addon[@tag]\n  gdam update @username/addon@tag\n  gdam install [--frozen-lockfile] [--offline] [--cache-dir DIR]\n  gdam publish @username/addon TAG [ASSET_NAME]\n  gdam remove @username/addon\n  gdam link @username/addon [local_path]\n  gdam unlink @username/addon\n  gdam unlink --all\n\nMachine results: gdam --json COMMAND [arguments] emits one JSON document; progress uses stderr.\n\nEnvironment:\n  GDAM_API_KEY      Clerk publishing key for manual publishing.\n  GDAM_API_URL      Registry API base url (default https://api.gdam.dev).\n  GITHUB_TOKEN      Optional GitHub token to avoid rate limits.",
    )
}
fn help() {
    if gdam::output::machine() {
        println!(
            "{}",
            serde_json::json!({"v":1,"ok":true,"result":{"help":usage()}})
        );
    } else {
        eprint!("{}", usage());
    }
}
#[derive(Default)]
struct Args {
    positionals: Vec<String>,
    install: install::Options,
    all: bool,
    help: bool,
}
fn parse(command: &str, args: Vec<String>) -> Result<Args> {
    let mut out = Args::default();
    let mut iter = args.into_iter();
    let mut flags = true;
    while let Some(arg) = iter.next() {
        if flags && arg == "--" {
            flags = false;
            continue;
        }
        if !flags || !arg.starts_with('-') || arg == "-" {
            flags = false;
            out.positionals.push(arg);
            continue;
        }
        let flag = arg.trim_start_matches('-');
        let (name, value) = flag
            .split_once('=')
            .map(|(a, b)| (a, Some(b)))
            .unwrap_or((flag, None));
        let boolean = |v: Option<&str>| -> Result<bool> {
            match v {
                None | Some("true" | "1" | "t" | "T" | "TRUE" | "True") => Ok(true),
                Some("false" | "0" | "f" | "F" | "FALSE" | "False") => Ok(false),
                _ => Err(gdam::input("invalid boolean flag")),
            }
        };
        match (command, name) {
            ("install", "frozen-lockfile") => out.install.frozen = boolean(value)?,
            ("install", "offline") => out.install.offline = boolean(value)?,
            ("install", "cache-dir") => {
                let v = value
                    .map(str::to_owned)
                    .or_else(|| iter.next())
                    .ok_or_else(|| gdam::input("cache-dir requires a value"))?;
                out.install.cache = PathBuf::from(v);
            }
            ("unlink", "all") => out.all = boolean(value)?,
            (_, "help" | "h") => {
                out.help = true;
                return Ok(out);
            }
            _ => {
                return Err(gdam::input(format!(
                    "flag provided but not defined: -{name}"
                )));
            }
        }
    }
    Ok(out)
}
fn run() -> Result<u8> {
    let mut args = std::env::args_os()
        .map(|s| {
            s.into_string()
                .map_err(|_| gdam::input("arguments must be UTF-8"))
        })
        .collect::<Result<Vec<_>>>()?;
    args.remove(0);
    let machine = args.first().is_some_and(|arg| arg == "--json");
    if machine {
        args.remove(0);
    }
    gdam::output::select_machine(machine);
    if args.is_empty() {
        if machine {
            return Err(gdam::input("expected a command after --json"));
        }
        help();
        return Ok(2);
    }
    let command = args.remove(0);
    if matches!(
        command.as_str(),
        "help" | "--help" | "-h" | "version" | "--version" | "-v" | "--build-info"
    ) && !args.is_empty()
    {
        return Err(gdam::input("unexpected arguments after help or version"));
    }
    match command.as_str() {
        "help" | "--help" | "-h" => {
            help();
            return Ok(0);
        }
        "version" | "--version" | "-v" => {
            if machine {
                println!(
                    "{}",
                    serde_json::json!({"v":1,"ok":true,"result":{"version":env!("GDAM_VERSION"),"commit":env!("GDAM_COMMIT"),"date":env!("GDAM_DATE")}})
                );
            } else {
                println!(
                    "gdam {}\ncommit: {}\nbuilt: {}",
                    env!("GDAM_VERSION"),
                    env!("GDAM_COMMIT"),
                    env!("GDAM_DATE")
                );
            }
            return Ok(0);
        }
        "--build-info" => {
            use sha2::{Digest, Sha256};
            println!(
                "{}",
                serde_json::json!({"runtime":"rust","version":env!("GDAM_VERSION"),"source_commit":env!("GDAM_COMMIT"),"target":env!("GDAM_TARGET"),"lock_sha256":format!("{:x}",Sha256::digest(include_bytes!("../Cargo.lock")))})
            );
            return Ok(0);
        }
        _ => {}
    }
    gdam::require(
        [
            "init", "add", "update", "remove", "rm", "link", "unlink", "install", "publish",
        ]
        .contains(&command.as_str()),
        format!("unknown command: {command}"),
    )?;
    let parsed = parse(&command, args)?;
    if parsed.help {
        help();
        return Ok(0);
    }
    let p = &parsed.positionals;
    let valid = match command.as_str() {
        "init" | "install" => p.is_empty(),
        "link" => [1, 2].contains(&p.len()),
        "publish" => [2, 3].contains(&p.len()),
        "unlink" => {
            if parsed.all {
                p.is_empty()
            } else {
                p.len() == 1
            }
        }
        _ => p.len() == 1,
    };
    if !valid {
        if !machine {
            eprint!("{}", usage());
        }
        return Err(gdam::input(format!("invalid arguments for {command}")));
    }
    if command == "init" {
        let cwd = std::env::current_dir()?;
        if let Some(root) = gdam::transaction::find(&cwd) {
            gdam::require(
                root.join("gdam.toml").exists(),
                "legacy gdam.json or recovery state requires explicit conversion to gdam.toml before init; legacy JSON v1 must be converted manually; no automatic import",
            )?;
        }
        if project::find_up(&cwd, "gdam.toml").is_none() {
            let dir = project::find_up(&cwd, "project.godot").unwrap_or_else(|| cwd.clone());
            let path = dir.join("gdam.toml");
            Manifest::default().save(&path)?;
            gdam::progress!("created {}", path.display());
        }
        if machine {
            println!(
                "{}",
                serde_json::json!({"v":1,"ok":true,"result":{"command":"init","root":project::find_up(&cwd,"gdam.toml")}})
            );
        }
        return Ok(0);
    }
    let seconds = match command.as_str() {
        "install" => 600,
        "add" | "update" | "unlink" => 300,
        _ => 30,
    };
    let network = Network::new(Duration::from_secs(seconds))?;
    if command == "publish" {
        commands::publish(&network, &p[0], &p[1], p.get(2).map(String::as_str))?;
        if machine {
            println!(
                "{}",
                serde_json::json!({"v":1,"ok":true,"result":{"command":"publish","addon":p[0],"tag":p[1],"published":true}})
            );
        }
        return Ok(0);
    }
    let result = Project::with_mutation(|project| {
        match command.as_str() {
            "add" | "update" => commands::add(project, &network, &p[0])?,
            "remove" | "rm" => commands::remove(project, &network, &p[0])?,
            "link" => commands::link(project, &network, &p[0], p.get(1).map(String::as_str))?,
            "unlink" if parsed.all => commands::unlink_all(project, &network)?,
            "unlink" => commands::unlink(project, &network, &p[0])?,
            "install" => {
                install::install(project, &network, &parsed.install, &project.manifest()?)?
            }
            _ => unreachable!(),
        }
        network.check_deadline()?;
        Ok(serde_json::json!({"command":command,"root":project.dir,"manifest":project.manifest()?}))
    })?;
    if machine {
        println!("{}", serde_json::json!({"v":1,"ok":true,"result":result}));
    }
    Ok(0)
}
fn main() {
    gdam::output::select_machine(
        std::env::args_os()
            .nth(1)
            .is_some_and(|arg| arg == "--json"),
    );
    let code = match run() {
        Ok(code) => code,
        Err(error) => {
            let mut text = format!("{error:#}");
            for key in ["GDAM_API_KEY", "GITHUB_TOKEN"] {
                if let Ok(secret) = std::env::var(key)
                    && !secret.trim().is_empty()
                {
                    text = text.replace(secret.trim(), "[redacted]");
                }
            }
            if !text.is_empty() {
                eprintln!("{text}");
            }
            if gdam::output::machine() {
                println!(
                    "{}",
                    serde_json::json!({"v":1,"ok":false,"error":{"code":if error.downcast_ref::<gdam::UserInput>().is_some(){"invalid_usage"}else{"operation_failed"},"message":text}})
                );
            }

            if error.downcast_ref::<gdam::UserInput>().is_some() {
                2
            } else {
                1
            }
        }
    };
    std::process::exit(i32::from(code));
}
