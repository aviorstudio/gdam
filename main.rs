use gdam::{
    Result, commands, install, manifest::Manifest, network::Network, project, transaction::Project,
};
use std::{path::PathBuf, time::Duration};
fn help() {
    eprintln!(
        "gdam - Godot addon manager (GitHub addons installer)\n\nUsage:\n  gdam --version\n  gdam init\n  gdam add @username/addon[@tag]\n  gdam update @username/addon@tag\n  gdam install [--frozen-lockfile] [--offline] [--cache-dir DIR]\n  gdam publish @username/addon TAG [ASSET_NAME]\n  gdam remove @username/addon\n  gdam link @username/addon [local_path]\n  gdam unlink @username/addon\n  gdam unlink --all\n\nEnvironment:\n  GDAM_API_KEY      Clerk publishing key for manual publishing.\n  GDAM_API_URL      Registry API base url (default https://api.gdam.dev).\n  GITHUB_TOKEN      Optional GitHub token to avoid rate limits."
    );
}
#[derive(Default)]
struct Args {
    positionals: Vec<String>,
    install: install::Options,
    all: bool,
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
                help();
                return Err(gdam::input(""));
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
    if args.is_empty() {
        help();
        return Ok(2);
    }
    let command = args.remove(0);
    match command.as_str() {
        "help" | "--help" | "-h" => {
            help();
            return Ok(0);
        }
        "version" | "--version" | "-v" => {
            println!(
                "gdam {}\ncommit: {}\nbuilt: {}",
                env!("GDAM_VERSION"),
                env!("GDAM_COMMIT"),
                env!("GDAM_DATE")
            );
            return Ok(0);
        }
        "--build-info" => {
            use sha2::{Digest, Sha256};
            println!(
                "{}",
                serde_json::json!({"runtime":"rust","version":env!("GDAM_VERSION"),"source_commit":env!("GDAM_COMMIT"),"target":env!("GDAM_TARGET"),"lock_sha256":format!("{:x}",Sha256::digest(include_bytes!("Cargo.lock")))})
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
        help();
        return Ok(2);
    }
    if command == "init" {
        let cwd = std::env::current_dir()?;
        if project::find_up(&cwd, "gdam.json").is_none() {
            let dir = project::find_up(&cwd, "project.godot").unwrap_or(cwd);
            let path = dir.join("gdam.json");
            Manifest::default().save(&path)?;
            println!("created {}", path.display());
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
        return Ok(0);
    }
    Project::with_mutation(|project| {
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
        network.check_deadline()
    })?;
    Ok(0)
}
fn main() {
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
            if error.downcast_ref::<gdam::UserInput>().is_some() {
                2
            } else {
                1
            }
        }
    };
    std::process::exit(i32::from(code));
}
