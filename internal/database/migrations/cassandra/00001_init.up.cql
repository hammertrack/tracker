CREATE TABLE IF NOT EXISTS hammertrack.mod_messages_by_user_name (
  user_name text,
  channel_name text,
  at timestamp,
  messages list<text>,
  sub int,
  PRIMARY KEY (user_name, channel_name, at)
) WITH CLUSTERING ORDER BY (channel_name ASC, at DESC);

CREATE TABLE IF NOT EXISTS hammertrack.mod_messages_by_channel_name (
  channel_name text,
  month int,
  at timestamp,
  user_name text,
  messages list<text>,
  sub int,
  PRIMARY KEY ((channel_name, month), at)
) WITH CLUSTERING ORDER BY (at DESC);

-- small table (n=100-1000) to be read in its entirety with a constant partition
CREATE TABLE IF NOT EXISTS hammertrack.tracked_channels (
  shard_id int,
  user_name text,
  lang int,
  PRIMARY KEY(shard_id, user_name)
);
