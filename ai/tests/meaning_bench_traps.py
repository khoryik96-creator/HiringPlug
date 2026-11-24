"""Made-up candidates for the traps a recruiter noticed in search by meaning
(test_meaning_traps.py; not a bench with an NDCG of its own): titles that
share a word with another field, the two kinds of Account Executive, a PA
and the CEO they work for, a title with a word that can describe work. Same
form as meaning_bench_a.py."""

# fmt: off
# (id, [(title, start, end or None)], what they did, families)
C = [
    ("adm", [("Admin Executive", 2020, None)], "Office supplies, courier, petty cash, filing and facilities for a 50-staff office.", ["adm"]),
    ("dba", [("Database Administrator", 2019, None)], "Oracle and SQL Server databases: backups, tuning, replication and patching.", ["it"]),
    ("netadm", [("Network Administrator", 2018, None)], "Cisco switches, firewalls, VPN and LAN/WAN for 20 branches.", ["it"]),
    ("entry", [("Data Entry Clerk", 2021, None)], "Keyed invoices and customer records into the system; filing.", ["adm"]),
    ("analyst", [("Data Analyst", 2021, None)], "SQL and Power BI dashboards for sales; ad-hoc analysis.", ["data"]),
    ("palm", [("Palm Oil Mill Engineer", 2017, None)], "Ran the boilers, presses and sterilisers of a palm oil mill.", ["plantation"]),
    ("drill", [("Drilling Engineer", 2016, None)], "Well planning and drilling operations offshore.", ["og"]),
    ("telco", [("Telecommunications Engineer", 2019, None)], "Planned 4G and 5G radio sites and fibre links for a telco.", ["telco"]),
    ("mkt", [("Marketing Executive", 2021, None)], "Ran Facebook and Google ads, events and brochures for a property developer.", ["mkt"]),
    ("sdm", [("IT Service Delivery Manager", 2018, None)], "Ran the ITIL service desk and SLAs for outsourced IT clients.", ["it"]),
    ("log", [("Logistics Manager", 2017, None)], "Warehouses, transport and freight forwarders for a 3PL.", ["log"]),
    ("sales_ae", [("Account Executive", 2021, None)], "Sold SaaS subscriptions to SMEs; closed RM 1.2m new business.", ["sales"]),
    ("acc_ae", [("Account Executive", 2020, None)], "Full set of accounts, bank reconciliation and month-end closing.", ["acc"]),
    ("pa_ceo", [("Personal Assistant to CEO", 2019, None)], "Diary, travel, meetings and minutes for the chief executive.", ["adm.ea"]),
    ("ceo", [("Chief Executive Officer", 2018, None)], "Runs a 300-staff manufacturing group; P&L, strategy and the board.", ["gm"]),
    ("admin_asst", [("Admin Assistant", 2022, None)], "Filing, courier, office supplies and petty cash.", ["adm"]),
    ("ta", [("Talent Acquisition Executive", 2021, None)], "Hired 80 engineers a year: sourcing on LinkedIn, interviews and offers.", ["rec"]),
    ("cx", [("Customer Experience Manager", 2020, None)], "", ["cx"]),  # no CV: their title alone
]
# fmt: on
