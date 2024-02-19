# TODO

- [x] Make an ExitReason type that implements Error and also can support
      distingushing :normal, :shutdown, types
- [x] linked processes don't exit when the reason is :normal
- [x] Add support for local names to genserver
  - [x] genserver.StartLink should check if name is registered, return error
        if so, then spawn_link and register name
  - [x] All genservers need a parent, so add self PID to
        start fun
- [x] rewrite genserver.Continue to return an error and handle correctly in all
      callbacks where it's used.
- [x] rewrite genserver.Init to return an error
- [x] Rewrite Genserver Call tests to execute more quickly
- [x] Rewrite Supervisor tests to execute more quickly
- [ ] test Exit, ensure that kill, normal reasons do correct behavior
- [x] Add tests for App - maybe convert to GenServer?
- [ ] ChildSpec.Type seems superfluous; adding `supervisor.NewSupChildSpec`
      would accomplish the same thing (making the default shutdown `Infinity`)
- [ ] support new slog for erl logger
- [ ] support custom crash reporter.
- [x] make sure recurringtask handles panics in the initFun and taskFun
- [ ] BUG: genserver handlers need to recover from panic and call terminate
