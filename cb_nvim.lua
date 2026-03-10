-- import vim

-- vim.notify("fooo", vim.notify.levels.ERROR)

return {
  neotest = {
    golang = {
      runner = "gotestsum",
    },
  },
}

