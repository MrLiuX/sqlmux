-- Seed for the SQLite tests (M14 F14.1), the cases of pg.sql in SQLite's
-- terms. The tests run it into a file of their own; there is no server to
-- load it into. Values are fixed, not now() or random, so assertions stay
-- stable.

-- single-column primary key: an INTEGER PRIMARY KEY is the rowid itself
create table t_user (
    id         integer primary key,
    name       text not null,
    email      text,
    active     integer not null default 1,
    created_at text not null default '2026-01-01 00:00:00'
);
insert into t_user (name, email) values ('alice', 'alice@example.com'), ('bob', null), ('carol', 'carol@example.com');

-- over 5000 rows, for paging and counting; one column per kind of value
create table t_order (
    id         integer primary key,
    user_id    integer not null references t_user (id),
    status     text not null default 'pending',
    amount     real not null,
    paid       integer,
    note       text,
    raw        blob,
    created_at text not null default '2026-01-01 00:00:00'
);
with recursive n (i) as (select 1 union all select i + 1 from n where i < 6000)
insert into t_order (user_id, status, amount, paid, note)
select 1 + i % 3, case i % 5 when 0 then 'pending' when 1 then 'queued' when 2 then 'running' when 3 then 'done' else 'failed' end,
       i * 1.5, i % 2, 'order ' || i
from n;
update t_order set raw = x'DEADBEEF00FF' where id = 1;

-- composite primary key
create table t_order_item (
    order_id integer not null references t_order (id),
    line_no  integer not null,
    sku_code text not null,
    qty      integer not null default 1,
    primary key (order_id, line_no)
);
insert into t_order_item values (1, 1, 'A-1', 2), (1, 2, 'B-7', 1), (2, 1, 'A-1', 5);

-- no primary key, only unique indexes: code and (title, code) can identify a
-- row, the nullable and expression ones can't (§8.4)
create table t_sku (
    code    text not null,
    barcode text,
    title   text not null,
    price   real
);
create unique index t_sku_code on t_sku (code);
create unique index t_sku_barcode on t_sku (barcode);
create unique index t_sku_lower_title on t_sku (lower(title));
insert into t_sku values ('S-1', '111', 'Widget', 9.5), ('S-2', null, 'Gadget', 20.0), ('S-3', null, 'Doohickey', null);

-- nothing to find a row by but its rowid
create table t_log (
    at   text not null,
    line text not null
);
insert into t_log values ('2026-01-01 00:00:01', 'started'), ('2026-01-01 00:00:02', 'started'), ('2026-01-01 00:00:03', 'stopped');

-- WITHOUT ROWID: the primary key is all there is
create table t_kv (
    k text primary key,
    v text
) without rowid;
insert into t_kv values ('a', '1'), ('b', null);

-- a generated column, a default, a column with no declared type, a blob
create table t_misc (
    id    integer primary key,
    w     integer not null,
    h     integer not null,
    area  integer generated always as (w * h) virtual,
    label text default 'none',
    free,
    data  blob
);
insert into t_misc (w, h, free, data) values (2, 3, 7, x'00FF'), (4, 5, 'text', 'plain text');

create view v_order_total as
select user_id, count(*) as n, sum(amount) as total from t_order group by user_id;
