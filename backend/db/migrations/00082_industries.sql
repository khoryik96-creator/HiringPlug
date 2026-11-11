-- Stable, tenant-scoped catalogue and multi-industry selections. Source IDs are provenance only.
-- +goose Up
CREATE TABLE industry_catalogue (
  tenant_id text PRIMARY KEY REFERENCES tenant(id),
  version integer NOT NULL DEFAULT 1 CHECK (version > 0)
);
CREATE TABLE industry (
  id text PRIMARY KEY DEFAULT new_id('industry') CHECK (id ~ '^industry_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id text NOT NULL DEFAULT app_current_tenant() REFERENCES tenant(id),
  parent_id text,
  label text NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 100),
  label_key text NOT NULL,
  aliases text[] NOT NULL DEFAULT '{}',
  retired boolean NOT NULL DEFAULT false,
  source_key text,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, source_key),
  UNIQUE (tenant_id, label_key),
  FOREIGN KEY (tenant_id, parent_id) REFERENCES industry(tenant_id, id),
  CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE TABLE industry_assignment (
  tenant_id text NOT NULL DEFAULT app_current_tenant() REFERENCES tenant(id),
  category_id text NOT NULL,
  candidate_id text REFERENCES candidate(id) ON DELETE CASCADE,
  company_id text REFERENCES company(id) ON DELETE CASCADE,
  is_primary boolean NOT NULL DEFAULT false,
  CHECK ((candidate_id IS NULL) <> (company_id IS NULL)),
  FOREIGN KEY (tenant_id, category_id) REFERENCES industry(tenant_id, id)
);
CREATE UNIQUE INDEX industry_assignment_candidate ON industry_assignment(tenant_id, candidate_id, category_id) WHERE candidate_id IS NOT NULL;
CREATE UNIQUE INDEX industry_assignment_company ON industry_assignment(tenant_id, company_id, category_id) WHERE company_id IS NOT NULL;
CREATE UNIQUE INDEX industry_primary_candidate ON industry_assignment(tenant_id, candidate_id) WHERE is_primary AND candidate_id IS NOT NULL;
CREATE UNIQUE INDEX industry_primary_company ON industry_assignment(tenant_id, company_id) WHERE is_primary AND company_id IS NOT NULL;
CREATE INDEX industry_assignment_category ON industry_assignment(tenant_id, category_id);
ALTER TABLE industry_catalogue ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON industry_catalogue USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE industry ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON industry USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE industry_assignment ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON industry_assignment USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

INSERT INTO industry_catalogue(tenant_id) SELECT id FROM tenant;
INSERT INTO industry(tenant_id,source_key,label,label_key)
SELECT t.id,v.source_key,v.label,v.label_key FROM tenant t CROSS JOIN (VALUES
('ind_9d3ff0d57ad4','Digital & E-Commerce','digital & e-commerce'),
('ind_fbe065970201','Financial Services','financial services'),
('ind_e5ef32d1f607','FMCG','fmcg'),
('ind_5d4f6b307762','Industrial/Manufacturing','industrial/manufacturing'),
('ind_539f6845962b','Information Technology & Services','information technology & services'),
('ind_a09c453435ec','Life Science/Medical','life science/medical'),
('ind_1087cb8e2a98','Property & Construction','property & construction'),
('ind_c4ede4b993b7','Professional Services','professional services'),
('ind_512ab3c6b986','Education','education'),
('ind_dc8f5679a75a','Government Sector','government sector')
) AS v(source_key,label,label_key);
INSERT INTO industry(tenant_id,source_key,label,label_key,parent_id)
SELECT t.id,v.source_key,v.label,v.label_key,p.id FROM tenant t CROSS JOIN (VALUES
('sub_79ea1243abb3','Digital – FinTech/Payment Getaway/E-Wallet','digital-fintech/payment getaway/e-wallet','ind_9d3ff0d57ad4'),
('sub_3ff979a1c22a','Digital – MarTech','digital-martech','ind_9d3ff0d57ad4'),
('sub_43e422b06a2a','Digital – EduTech','digital-edutech','ind_9d3ff0d57ad4'),
('sub_0aa85f61f5a6','Digital - InsurTech','digital-insurtech','ind_9d3ff0d57ad4'),
('sub_6dbeb0412909','Digital - HealthTech/MedTech','digital-healthtech/medtech','ind_9d3ff0d57ad4'),
('sub_b8da3c706a32','Digital - LogTech','digital-logtech','ind_9d3ff0d57ad4'),
('sub_128776b47b49','E-Commerce – Marketplace/Digital Platform','e-commerce-marketplace/digital platform','ind_9d3ff0d57ad4'),
('sub_dfbd3ba6fcb6','E-Commerce – Fulfillment/Enabler','e-commerce-fulfillment/enabler','ind_9d3ff0d57ad4'),
('sub_63e1f7ae7965','E-Commerce – Food & Grocery Delivery','e-commerce-food & grocery delivery','ind_9d3ff0d57ad4'),
('sub_f25238b7cfa6','FSI - Asset Management','fsi-asset management','ind_fbe065970201'),
('sub_d9102d124a15','FSI – Banking','fsi-banking','ind_fbe065970201'),
('sub_1970a7419148','FSI - Hedge/Mutual Funds','fsi-hedge/mutual funds','ind_fbe065970201'),
('sub_d482b4f7fb7f','FSI - Investment Bank','fsi-investment bank','ind_fbe065970201'),
('sub_97c5b77ac45d','FSI - Insurance','fsi-insurance','ind_fbe065970201'),
('sub_c917c485772e','FSI - Private Equity/Venture Capital','fsi-private equity/venture capital','ind_fbe065970201'),
('sub_3186b4bea4e0','FMCG – Consumer Durables','fmcg-consumer durables','ind_e5ef32d1f607'),
('sub_0d64c26c0afc','FMCG – Consumer Electronics','fmcg-consumer electronics','ind_e5ef32d1f607'),
('sub_04f080052502','FMCG – Food & Beverage','fmcg-food & beverage','ind_e5ef32d1f607'),
('sub_b70ba43a2011','FMCG – Fashion & Apparel/Accessories','fmcg-fashion & apparel/accessories','ind_e5ef32d1f607'),
('sub_c969ea635cbb','FMCG – Luxury Goods','fmcg-luxury goods','ind_e5ef32d1f607'),
('sub_cf05b3614d13','FMCG – MLM','fmcg-mlm','ind_e5ef32d1f607'),
('sub_cdc55a363f83','FMCG – Personal Care/Cosmetics','fmcg-personal care/cosmetics','ind_e5ef32d1f607'),
('sub_a12293753213','FMCG – Retail','fmcg-retail','ind_e5ef32d1f607'),
('sub_fb1dd0db3780','FMCG – Sporting Goods','fmcg-sporting goods','ind_e5ef32d1f607'),
('sub_2fe13bc0a515','Industrial/Manufacturing - Aerospace','industrial/manufacturing-aerospace','ind_5d4f6b307762'),
('sub_08e9dfaa6380','Agriculture – Agriculture Supply','agriculture-agriculture supply','ind_5d4f6b307762'),
('sub_105856674334','Agriculture – Crop/Plantation','agriculture-crop/plantation','ind_5d4f6b307762'),
('sub_d0ecfd727f4d','Agriculture – Livestock/Feed Mill','agriculture-livestock/feed mill','ind_5d4f6b307762'),
('sub_7458b014c547','Industrial/Manufacturing - Automation','industrial/manufacturing-automation','ind_5d4f6b307762'),
('sub_2e0fb58654ad','Industrial/Manufacturing - Automotive','industrial/manufacturing-automotive','ind_5d4f6b307762'),
('sub_99df751254a7','Chemicals – Food/Health/Additives','chemicals-food/health/additives','ind_5d4f6b307762'),
('sub_b1db34be13b1','Chemicals – Paint/Coat/Adhesive','chemicals-paint/coat/adhesive','ind_5d4f6b307762'),
('sub_85eed038df70','Chemicals – Plastic/Package/Print','chemicals-plastic/package/print','ind_5d4f6b307762'),
('sub_f0755dc8521e','Chemicals – Textiles','chemicals-textiles','ind_5d4f6b307762'),
('sub_c6879435f906','Distributions & Logistics','distributions & logistics','ind_5d4f6b307762'),
('sub_5c50b1199c9d','Industrial/Manufacturing - Diversified Manufacturing','industrial/manufacturing-diversified manufacturing','ind_5d4f6b307762'),
('sub_9b82d3f5c0f0','Industrial/Manufacturing - Electronic Components/EMS/CMS','industrial/manufacturing-electronic components/ems/cms','ind_5d4f6b307762'),
('sub_424f008bd660','Energy/Power/Utilities – EPC','energy/power/utilities-epc','ind_5d4f6b307762'),
('sub_d050b8ae1d7d','Energy/Power/Utilities – Mining','energy/power/utilities-mining','ind_5d4f6b307762'),
('sub_bdb43116e3e6','Oil & Gas – Upstream','oil & gas-upstream','ind_5d4f6b307762'),
('sub_067bddac7db7','Oil & Gas – DownStream','oil & gas-downstream','ind_5d4f6b307762'),
('sub_2cd6031c3f16','Energy/Power/Utilities - Renewable/Power','energy/power/utilities-renewable/power','ind_5d4f6b307762'),
('sub_be07dc4d7810','Industrial/Manufacturing - Heavy Industry/Equipment','industrial/manufacturing-heavy industry/equipment','ind_5d4f6b307762'),
('sub_2fa3cfcf0503','Industrial/Manufacturing - Marine & Shipping','industrial/manufacturing-marine & shipping','ind_5d4f6b307762'),
('sub_3b7ebbdd477b','Industrial/Manufacturing - Metalwork','industrial/manufacturing-metalwork','ind_5d4f6b307762'),
('sub_00c0d7294272','Industrial/Manufacturing - Railway & Transportation','industrial/manufacturing-railway & transportation','ind_5d4f6b307762'),
('sub_b951cee64042','Industrial/Manufacturing - Semiconductor','industrial/manufacturing-semiconductor','ind_5d4f6b307762'),
('sub_7efa9c11df97','Tech - Call Centre/BPO','tech-call centre/bpo','ind_539f6845962b'),
('sub_c2783b0867a9','Tech - Gaming','tech-gaming','ind_539f6845962b'),
('sub_336344ff3e0a','Tech – ERP/CRM/HCM/Application','tech-erp/crm/hcm/application','ind_539f6845962b'),
('sub_a8683b3da6ca','Tech - Hardware','tech-hardware','ind_539f6845962b'),
('sub_16463c95935a','Tech - Networks','tech-networks','ind_539f6845962b'),
('sub_a566814697e8','Tech - Security','tech-security','ind_539f6845962b'),
('sub_5190fe874a4a','Tech – Software/Software House','tech-software/software house','ind_539f6845962b'),
('sub_2f5af6f5b13d','Tech - Cloud/SaaS','tech-cloud/saas','ind_539f6845962b'),
('sub_f3db0b3f4d89','Tech - Sys Integrator/Svc Provider','tech-sys integrator/svc provider','ind_539f6845962b'),
('sub_bda89fa94ee7','Tech – Telecommunications','tech-telecommunications','ind_539f6845962b'),
('sub_7f2f2eba6993','Tech - Blockchain','tech-blockchain','ind_539f6845962b'),
('sub_6e5f631efa0b','Life Science/Medical - Biotechnology','life science/medical-biotechnology','ind_a09c453435ec'),
('sub_e593738e0703','Life Science/Medical - Hospitals/Nursing','life science/medical-hospitals/nursing','ind_a09c453435ec'),
('sub_0567e7f5a373','Life Science/Medical - Labs/CROs','life science/medical-labs/cros','ind_a09c453435ec'),
('sub_c5a136c1ac8e','Life Science/Medical - Medical devices/Equipment','life science/medical-medical devices/equipment','ind_a09c453435ec'),
('sub_4fb38070a488','Life Science/Medical - OTC/Healthcare','life science/medical-otc/healthcare','ind_a09c453435ec'),
('sub_5e46ae2db7ce','Life Science/Medical - Pharmaceutical','life science/medical-pharmaceutical','ind_a09c453435ec'),
('sub_8331150739e9','Life Science/Medical - Veterinary','life science/medical-veterinary','ind_a09c453435ec'),
('sub_c034c54835e0','P&C – Architecture','p&c-architecture','ind_1087cb8e2a98'),
('sub_a52972a22cd4','P&C – Building Materials','p&c-building materials','ind_1087cb8e2a98'),
('sub_2b0ca2a4c7b1','P&C – Construction','p&c-construction','ind_1087cb8e2a98'),
('sub_f3a3091436b1','P&C – General Practice','p&c-general practice','ind_1087cb8e2a98'),
('sub_b53566362dd7','P&C – Hospitality & Tourism','p&c-hospitality & tourism','ind_1087cb8e2a98'),
('sub_f1073c2f0012','P&C – Interior Design','p&c-interior design','ind_1087cb8e2a98'),
('sub_af00efde8c59','P&C – Property Development','p&c-property development','ind_1087cb8e2a98'),
('sub_dbfaa0553c0c','P&C – Real Estate Consultancy','p&c-real estate consultancy','ind_1087cb8e2a98'),
('sub_3b774a7c87bd','P&C – REITS','p&c-reits','ind_1087cb8e2a98'),
('sub_5b489022cbc8','Professional Services - Accounting Outsourcing','professional services-accounting outsourcing','ind_c4ede4b993b7'),
('sub_cef426b07d7c','Professional Services- Advertising & Marketing','professional services-advertising & marketing','ind_c4ede4b993b7'),
('sub_7fad13318b12','Professional Services - Corporate Secretary','professional services-corporate secretary','ind_c4ede4b993b7'),
('sub_607dffe3a212','Professional Services - Financial Advisory','professional services-financial advisory','ind_c4ede4b993b7'),
('sub_d68aed632a81','Professional Services - IT Outsourcing','professional services-it outsourcing','ind_c4ede4b993b7'),
('sub_fdf295db08c7','Professional Services - Recruitment & Staffing','professional services-recruitment & staffing','ind_c4ede4b993b7'),
('sub_5b37bf8f113d','Professional Services - Consulting','professional services-consulting','ind_c4ede4b993b7')
) AS v(source_key,label,label_key,parent_key)
JOIN industry p ON p.tenant_id=t.id AND p.source_key=v.parent_key;

-- +goose StatementBegin
CREATE FUNCTION app_industry_key(value text) RETURNS text LANGUAGE sql IMMUTABLE STRICT AS $$
 SELECT lower(regexp_replace(regexp_replace(translate(normalize(btrim(value), NFC), '‐‑‒–—―−', '-------'), '\s*-\s*', '-', 'g'), '\s+', ' ', 'g'));
$$;
-- +goose StatementEnd
INSERT INTO industry(tenant_id,label,label_key,aliases,retired,source_key)
SELECT tenant_id,label,app_industry_key(label),ARRAY[key],hidden,'legacy:' || key
FROM field_option WHERE list='industry'
ON CONFLICT(tenant_id,label_key) DO NOTHING;
UPDATE industry i SET aliases = array_append(i.aliases,f.key), retired=f.hidden
FROM field_option f WHERE f.list='industry' AND f.tenant_id=i.tenant_id AND app_industry_key(f.label)=i.label_key
AND NOT f.key=ANY(i.aliases);
-- Unknown historical labels remain readable without being offered for new assignments.
INSERT INTO industry(tenant_id,label,label_key,retired,source_key)
SELECT DISTINCT r.tenant_id,r.label,app_industry_key(r.label),true,'legacy-value:' || app_industry_key(r.label) FROM (
 SELECT tenant_id,industry AS label FROM candidate WHERE industry IS NOT NULL
 UNION SELECT tenant_id,industry FROM company WHERE industry IS NOT NULL
) r WHERE NOT EXISTS(SELECT 1 FROM industry i WHERE i.tenant_id=r.tenant_id AND (i.label_key=app_industry_key(r.label) OR r.label=ANY(i.aliases)))
ON CONFLICT(tenant_id,label_key) DO NOTHING;
INSERT INTO industry_assignment(tenant_id,category_id,candidate_id,is_primary)
SELECT c.tenant_id,i.id,c.id,true FROM candidate c JOIN LATERAL (SELECT * FROM industry x WHERE x.tenant_id=c.tenant_id AND (x.label_key=app_industry_key(c.industry) OR c.industry=ANY(x.aliases)) ORDER BY (x.label_key=app_industry_key(c.industry)) DESC, x.id LIMIT 1) i ON true;
INSERT INTO industry_assignment(tenant_id,category_id,company_id,is_primary)
SELECT c.tenant_id,i.id,c.id,true FROM company c JOIN LATERAL (SELECT * FROM industry x WHERE x.tenant_id=c.tenant_id AND (x.label_key=app_industry_key(c.industry) OR c.industry=ANY(x.aliases)) ORDER BY (x.label_key=app_industry_key(c.industry)) DESC, x.id LIMIT 1) i ON true;

-- Keep the scalar compatibility view aligned with the new stable-ID selections.
UPDATE candidate c SET industry=coalesce(p.label,i.label),version=c.version+1
FROM industry_assignment a JOIN industry i ON i.id=a.category_id LEFT JOIN industry p ON p.id=i.parent_id
WHERE a.candidate_id=c.id AND a.is_primary AND c.industry IS DISTINCT FROM coalesce(p.label,i.label);
UPDATE company c SET industry=coalesce(p.label,i.label),version=c.version+1
FROM industry_assignment a JOIN industry i ON i.id=a.category_id LEFT JOIN industry p ON p.id=i.parent_id
WHERE a.company_id=c.id AND a.is_primary AND c.industry IS DISTINCT FROM coalesce(p.label,i.label);

-- +goose Down
DROP TABLE industry_assignment;
DROP TABLE industry;
DROP TABLE industry_catalogue;
DROP FUNCTION app_industry_key(text);
