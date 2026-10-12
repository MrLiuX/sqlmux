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
insert into t_order (user_id, status, amount, paid)
select 1 + i % 3, case i % 5 when 0 then 'pending' when 1 then 'queued' when 2 then 'running' when 3 then 'done' else 'failed' end,
       i * 1.5, i % 2
from n;
update t_order set raw = x'DEADBEEF00FF' where id = 1;

create index t_order_user_status on t_order (user_id, status);

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

-- a foreign key with no column written: it points at the parent's primary key
create table t_note (
    id      integer primary key,
    user_id integer references t_user
);

-- F14.1 review: what SQLite does its own way

-- a partial unique index is no key; a blob is no key to find a row by
create table t_part (a text not null, b text not null);
create unique index t_part_a on t_part (a) where b = 'x';
insert into t_part values ('1', 'x'), ('1', 'y');
create table t_bk (id blob(16) primary key, name text);
insert into t_bk values (x'00112233445566778899aabbccddeeff', 'uuid');

-- a primary key in another order than the columns; a key with no declared type,
-- which may hold an integer or a text; INT and DESC are no alias of the rowid
create table t_ck (b text, a integer, primary key (a, b));
create table t_free (k primary key, v text);
insert into t_free values (1, 'one'), ('two', 'two'), (0, 'zero');
create table t_free_wr (k primary key, v text) without rowid;
insert into t_free_wr values (1, 'one'), ('two', 'two'), (0, 'zero');
create table t_int (id int primary key, v text);
create table t_desc (id integer primary key desc, v text);

-- the columns SQLite reads as times, one written another way
create table t_dt (id integer primary key, at datetime, d date, ts timestamp, label text);
insert into t_dt values (1, '2026-01-02 03:04:05', '2026-01-02', '2026-01-02 03:04:05.250', 'standard'),
                        (2, '2026-01-02T03:04:05Z', null, null, 'T and Z');

-- reals that need all their digits, and the ends of how they are written
create table t_real (id integer primary key, x real);
insert into t_real values (1, 1.0 / 7), (2, 0.1 + 0.2), (3, 1e16), (4, 1e17), (5, 123456789.123456789), (6, 1e-5), (7, 100.0);

-- generated columns, stored and virtual; a column called rowid
create table t_gen (a integer, b integer generated always as (a * 2) stored, c integer as (a + 1));
create table t_rowid (rowid text, v text);

-- a trigger: its body is one statement, ;s and all; a virtual table with its shadow tables
create table t_aud (n integer);
create trigger t_aud_ins after insert on t_aud begin
    insert into t_log values ('aud', 'row ' || new.n);
    insert into t_log values ('aud', 'again');
end;
create virtual table ft using fts5 (body);
