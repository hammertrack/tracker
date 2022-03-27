BEGIN;

DO $$ BEGIN
	CREATE TYPE langISO31661 AS ENUM (
	'aa','hy','or','ab','hz','os','af','id','pa','ak','ig','pl',
	'am','ii','ps','an','ik','pt','ar','io','qu','as','is','rm',
	'av','it','rn','ay','iu','ro','az','ja','ru','ba','jv','rw',
	'be','ka','sa','bg','kg','sc','bh','ki','sd','bi','kj','se'	,
	'bm','kk','sg','bn','kl','si','bo','km','sk','br','kn','sl',
	'bs','ko','sm','ca','kr','sn','ce','ks','so','ch','ku','sq',
	'co','kv','sr','cr','kw','ss','cs','ky','st','cv','lb','su',
	'cy','lg','sv','da','li','sw','de','ln','ta','dv','lo','te',
	'dz','lt','tg','ee','lu','th','el','lv','ti','en','mg','tk',
	'es','mh','tl','et','mi','tn','eu','mk','to','fa','ml','tr',
	'ff','mn','ts','fi','mr','tt','fj','ms','tw','fo','mt','ty',
	'fr','my','ug','fy','na','uk','ga','nb','ur','gd','nd','uz',
	'gl','ne','ve','gn','ng','vi','gu','nl','wa','gv','nn','wo',
	'ha','no','xh','he','nr','yi','hi','nv','yo','ho','ny','za',
	'hr','oc','zh','ht','oj','zu','hu','om','other'
	);

  CREATE TYPE ban_type AS ENUM (
  'ban', 'timeout', 'deletion'
  );
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS broadcaster (
  broadcaster_id serial PRIMARY KEY,
  name varchar(25) NOT NULL,
  lang langISO31661 NOT NULL
);

CREATE TABLE IF NOT EXISTS clearchat (
  clearchat_id bigserial PRIMARY KEY,
  type ban_type NOT NULL,
  username varchar(25) NOT NULL,
  channel_name varchar(25) NOT NULL,
  channel_id int NOT NULL REFERENCES broadcaster,
  duration int NOT NULL,
  at timestamp NOT NULL,
  messages varchar NOT NULL
);


-- For queries: filter by types
CREATE INDEX idx_clearchat_type
ON clearchat(type);

-- For queries: most recent of a particular channel
CREATE INDEX idx_clearchat_channel_at
ON clearchat(channel_name, at DESC);

-- For queries: most recent of a particular user
CREATE INDEX idx_clearchat_username_at
ON clearchat(username, at DESC);

INSERT INTO broadcaster(name, lang)
VALUES ('queryselectorall', 'es'),
('zeling', 'es');

COMMIT;
