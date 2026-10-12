-- Seed for the MySQL integration tests and e2e (M5 F5.4), the cases of
-- pg.sql in MySQL's terms. The mysql image loads it on first start, as
-- root, in the sqlmux database. Values are fixed, not now() or rand(), so
-- screens and assertions stay stable.

-- a second database, as pg.sql's second schema
create database agentable;
grant all on agentable.* to 'sqlmux'@'%';
-- the tests' and e2e's databases of their own, sqlmux_<who>_<pid> (AGENTS.md)
grant all on `sqlmux\_%`.* to 'sqlmux'@'%';
-- the definitions of the functions, which a grant on a database does not show (F14.3)
grant show_routine on *.* to 'sqlmux'@'%';

-- the inserts below count to 6000
set session cte_max_recursion_depth = 10000;

-- single-column primary key
create table t_user (
    id         bigint auto_increment primary key,
    name       varchar(64) not null,
    email      varchar(128),
    active     boolean not null default true,
    created_at datetime not null default current_timestamp
);

-- over 5000 rows, for paging and counting; one column per type the grid colours
create table t_order (
    id         bigint auto_increment primary key,
    user_id    bigint not null,
    status     enum('pending', 'queued', 'running', 'done', 'failed') not null default 'pending',
    amount     decimal(10, 2) not null,
    paid       boolean,
    meta       json,
    raw        json,
    note       text,
    created_at datetime not null default current_timestamp,
    deleted_at timestamp null,
    foreign key (user_id) references t_user (id)
);

-- composite primary key
create table t_order_item (
    order_id bigint not null,
    line_no  int not null,
    sku_code varchar(64) not null,
    qty      int not null default 1,
    primary key (order_id, line_no),
    foreign key (order_id) references t_order (id)
);

-- no primary key, only unique indexes. code and (title, code), keyed out of
-- column order, can identify a row; the nullable and expression ones can't
-- (§8.4). MySQL has no partial index nor INCLUDE.
create table t_sku (
    code    varchar(64) not null,
    barcode varchar(64),
    title   varchar(64) not null,
    price   decimal(10, 2)
);
create unique index t_sku_code on t_sku (code);
create unique index t_sku_barcode on t_sku (barcode);
create unique index t_sku_lower_title on t_sku ((lower(title)));
create unique index t_sku_title_code on t_sku (title, code);

-- no primary key and no unique index; one row carries control characters (§7.6)
create table t_log (
    logged_at datetime not null,
    msg       text
);

create view v_paid_order as
    select id, user_id, amount from t_order where paid;

-- partitions are no tables of their own (§8.4); the primary key's order is
-- not the columns' order
create table t_event (
    id          bigint not null,
    occurred_at datetime not null,
    kind        varchar(32) not null,
    primary key (occurred_at, id)
) partition by range columns (occurred_at) (
    partition t_event_2025 values less than ('2026-01-01'),
    partition t_event_2026 values less than ('2027-01-01')
);

create table agentable.agent (
    id   int auto_increment primary key,
    name varchar(64) not null unique
);

create table agentable.agent_version (
    agent_id int not null,
    version  int not null,
    released boolean not null default false,
    primary key (agent_id, version),
    foreign key (agent_id) references agentable.agent (id)
);

create table agentable.goal (
    id       int auto_increment primary key,
    agent_id int,
    title    varchar(64) not null,
    foreign key (agent_id) references agentable.agent (id)
);

insert into t_user (name, email, active, created_at)
with recursive s (i) as (select 1 union all select i + 1 from s where i < 50)
select concat('user_', i),
       case when i % 5 = 0 then null else concat('user_', i, '@example.com') end,
       i % 7 <> 0,
       timestamp '2026-01-01 00:00:00' + interval i day
from s;

insert into t_order (user_id, status, amount, paid, meta, raw, note, created_at, deleted_at)
with recursive s (i) as (select 1 union all select i + 1 from s where i < 6000)
select 1 + i % 50,
       elt(1 + i % 4, 'pending', 'running', 'done', 'failed'),
       (i % 1000) + 0.99,
       case when i % 3 = 0 then null else i % 2 = 0 end,
       json_object('n', i, 'tags', json_array('a', 'b')),
       json_object('n', i),
       case when i % 10 = 0 then concat('note ', i) end,
       timestamp '2026-09-01 00:00:00' + interval i minute,
       case when i % 100 = 0 then timestamp '2026-09-20 12:00:00' end
from s;

insert into t_order_item (order_id, line_no, sku_code, qty)
with recursive o (n) as (select 1 union all select n + 1 from o where n < 200),
               l (n) as (select 1 union all select n + 1 from l where n < 2)
select o.n, l.n, concat('sku_', (o.n + l.n) % 20), 1 + o.n % 3
from o, l;

insert into t_sku (code, barcode, title, price)
with recursive s (i) as (select 0 union all select i + 1 from s where i < 19)
select concat('sku_', i),
       case when i % 4 = 0 then null else concat('69', lpad(i, 11, '0')) end,
       concat('SKU ', i),
       case when i % 5 = 0 then null else i * 10 end
from s;

insert into t_log (logged_at, msg) values
    ('2026-09-21 10:00:00', 'plain'),
    ('2026-09-21 10:01:00', concat('line one\nline two\tafter tab ', char(27 using utf8mb4), '[31mred', char(27 using utf8mb4), '[0m end')),
    ('2026-09-21 10:02:00', null);

insert into t_event (id, occurred_at, kind) values
    (1, '2025-06-01 00:00:00', 'signup'),
    (2, '2025-12-31 23:59:59', 'login'),
    (3, '2026-02-01 08:00:00', 'login');

insert into agentable.agent (name) values ('planner'), ('coder'), ('tester');
insert into agentable.agent_version (agent_id, version, released) values
    (1, 1, true), (1, 2, false), (2, 1, true), (3, 1, false);
insert into agentable.goal (agent_id, title) values
    (1, 'ship m1'), (2, 'write tests'), (null, 'unassigned');

-- M14 F14.3: a plain index and functions, one in the second database
create index t_order_user_created on t_order (user_id, created_at);
create function fn_order_total(order_id bigint) returns decimal(10, 2) deterministic reads sql data
    return (select amount from t_order where id = order_id);
create function fn_greet(who varchar(64)) returns varchar(80) deterministic no sql
    return concat('hello ', who);
create function agentable.fn_agent_count() returns bigint reads sql data
    return (select count(*) from agentable.agent);

analyze table t_user, t_order, t_order_item, t_sku, t_log, t_event,
    agentable.agent, agentable.agent_version, agentable.goal;
