# TODO
- Data is pulled every x minutes, change that to a configurable value, use 
  an environment variable named DURBO_DATA_PULL_MINUTES, or a parameter,
  the default, if the value is not set can be five minutes.
- Add a config file for filtering where I can add expense categories to be not included
  in the pie chart, and add another part to this config to add recurring vendors
  and budgets for those vendors
- Show the recurrning vendors as a single tile that shows a count and a montly total
  for all recurring vendors