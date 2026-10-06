-- Hodir (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_hodir.cpp
-- (boss_hodir (CreatureScript) via GetUlduarAI<boss_hodirAI>
-- (BossAI), BOSS_HODIR = 7 — registered from inside
-- AddSC_boss_hodir(); loader decl 119 / call 314 per
-- northrend_script_loader.cpp — the TENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_mimiron() (decl 118 / call 313); the call
-- after it is AddSC_boss_freya() — loader order confirmed
-- 2026-10-06 in the 23:03 EDT audit (this lua was written
-- from the Oct 2 port session and re-audited, per the
-- xt002 22:38 / vezax 22:48 / assembly_of_iron 22:54 /
-- kologarn 22:58 stale-header precedent — this run fixes the
-- stale first-pass "No hodir lua existed" line to this
-- audited-artifact wording).
-- Entry: 32845 Hodir (ulduar.h NPC_HODIR, line 78 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the twelve script names ("boss_hodir", "npc_icicle",
-- "npc_snowpacked_icicle", "npc_hodir_priest",
-- "npc_hodir_shaman", "npc_hodir_druid", "npc_hodir_mage",
-- "npc_toasty_fire", "npc_ice_block", "npc_flash_freeze",
-- "spell_biting_cold", "spell_biting_cold_dot") hits
-- boss_hodir.cpp (+ the loader decl/call lines for
-- AddSC_boss_hodir) only; zero sql/ hits for all twelve.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied,
-- 9 OnDamageTaken.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the DoCast
-- SPELL_BITING_COLD leg and the six ScheduleEvent legs
-- have no bridges — the auriaya engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — at least the
-- forty-first player-gated variant ported: 40 player-gated
-- lua ports carry mtimes untouched since before this file's
-- creation (file-birth order puts it at sixtieth; both
-- counts floor the original "twenty-fifth" claim, which was
-- also duplicated verbatim on boss_general_vezax.lua).
-- DamageTaken — lethal (damage >= health) -> Talk(SAY_DEATH
-- 4) + damage = 0 (event 9; the numeric second return
-- rewrites damage C++-exact — the moroes damage-rewrite
-- precedent; the companion legs have no bridges — see
-- DOCUMENTED-ONLY).
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entry 32845):
-- DamageTaken companion legs — RemoveAllAuras /
-- RemoveAllAttackers / AttackStop / SetReactState
-- REACT_PASSIVE / SetFlag UNIT_FLAG_NON_ATTACKABLE /
-- SetControlled root / InterruptNonMeleeSpells /
-- StopMoving / MoveIdle / SetControlled stun /
-- CombatStop — no bridges; DoCastAOE SPELL_KILL_CREDIT
-- 64899 — no cast bridge; SetFaction FACTION_FRIENDLY —
-- no faction bridge; DespawnOrUnsummon(10s) — no despawn
-- bridge; _JustDied() passthrough — never reachable in Go
-- since the boss cannot actually die while the damage=0
-- rewrite holds (consequence of the rewrite without the
-- despawn leg: every subsequent hit still satisfies
-- damage >= health, so the death Talk re-fires on each
-- hit — verbatim C++ behavior, which in C++ is bounded
-- by the 10s despawn; documented here since Go cannot
-- despawn him).
-- instance->SetData(DATA_HODIR_RARE_CACHE, 1) on lethal
-- DamageTaken and DATA_HODIR_RARE_CACHE = 0 on
-- EVENT_RARE_CACHE — no instance SetData bridge; the
-- iCouldSayThatThisCacheWasRare flag joins the
-- no-SetData-bridge queue.
-- Reset — _Reset() + SetReactState REACT_PASSIVE +
-- frozen-helper SummonCreature legs — no bridges.
-- UpdateAI timer Talks — Talk(SAY_FLASH_FREEZE 2) +
-- Talk(EMOTE_FREEZE 7) (EVENT_FLASH_FREEZE),
-- Talk(SAY_STALACTITE 3) + Talk(EMOTE_BLOWS 8)
-- (EVENT_BLOWS), Talk(SAY_HARD_MODE_FAILED 6)
-- (EVENT_RARE_CACHE), Talk(SAY_BERSERK 5)
-- (EVENT_BERSERK) — the timer event machine has no
-- bridge; all join the no-timer-bridge queue.
-- npc_hodir_priest / npc_hodir_shaman / npc_hodir_druid /
-- npc_hodir_mage JustDied -> instance->GetCreature
-- (BOSS_HODIR)->AI()->DoAction
-- (ACTION_I_HAVE_THE_COOLEST_FRIENDS) — no DoAction
-- bridge; their DoSpellAttackIfReady combat legs have no
-- cast bridges — no registrations.
-- npc_flash_freezeAI DamageTaken — flash-freeze helper
-- teleport / SPELL_FLASH_FREEZE_HELPER legs — no
-- bridges; npc_icicle / npc_snowpacked_icicle fall
-- damage / snowdrift legs — no bridges; npc_toasty_fire
-- (GO_TOASTY_FIRE 194300), npc_ice_block — no bridges;
-- no registrations.
-- spell_biting_cold / spell_biting_cold_dot —
-- SpellScript / AuraScript — no SpellScript / AuraScript
-- bridges; both join the no-SpellScript-bridge queue.
-- Achievement data legs (DATA_GETTING_COLD_IN_HERE /
-- ACTION_CHEESE_THE_FREEZE /
-- ACTION_I_HAVE_THE_COOLEST_FRIENDS /
-- DATA_HODIR_RARE_CACHE — the C++ @todo names Storm
-- Cloud and Toasty Fires) — no GetData / DoAction /
-- achievement bridges; all join the unmodeled-achievement
-- queue.

local ENTRY_HODIR = 32845

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 4

-- C++ JustEngagedWith: BossAI passthrough + DoCast
-- SPELL_BITING_COLD + six ScheduleEvent legs — only the
-- Talk arm is bridgeable.
local function hodirEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function hodirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ DamageTaken: damage >= me->GetHealth() -> damage = 0
-- + Talk(SAY_DEATH) — only the Talk arm and the damage
-- rewrite are bridgeable; the numeric return performs the
-- C++-exact damage=0 rewrite (the moroes damage-rewrite
-- precedent).
local function hodirDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        creature:Talk(SAY_DEATH)
        return 0
    end
end

RegisterCreatureEvent(ENTRY_HODIR, 1, hodirEnterCombat)
RegisterCreatureEvent(ENTRY_HODIR, 3, hodirTargetDied)
RegisterCreatureEvent(ENTRY_HODIR, 9, hodirDamageTaken)
