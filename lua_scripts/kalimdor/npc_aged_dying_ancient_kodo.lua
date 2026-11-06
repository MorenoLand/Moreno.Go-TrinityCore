-- Aged / Dying / Ancient Kodo (Desolace) --
-- Lua port of src/server/scripts/Kalimdor/zone_desolace.cpp
-- (npc_aged_dying_ancient_kodoAI — SpellHit buff arms + gossip hello).
-- Zone-script unit per kalimdor_script_loader.cpp order
-- (ashenvale, azshara, azuremyst_isle, bloodmyst_isle,
-- darkshore closed; desolace now).
-- Entries (DyingKodo enum, verifiable from the C++ sources):
-- 4700 (NPC_AGED_KODO) / 4701 (NPC_DYING_KODO) /
-- 4702 (NPC_ANCIENT_KODO). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 14 OnSpellHit (the 18153 buff arm +
-- the 18362 gossip arm); gossip event 1 OnGossipHello.
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention); the two modeled arms are triggered self-casts.
-- Fight shape (C++-exact for the modeled arms): OnSpellHit(14)
-- by SPELL_KODO_KOMBO_ITEM 18153 -> if the entry is one of the
-- three kodo entries and the caster lacks SPELL_KODO_KOMBO_
-- PLAYER_BUFF 18172: triggered self-cast 18172 on the caster
-- (modeled as caster:AddAura — player:CastSpell bridged
-- 2026-10-24, AddAura stands, brutallus precedent) + triggered
-- self-cast 18377 on the kodo.
-- The C++ also gates on the kodo not having SPELL_KODO_KOMBO_
-- DESPAWN_BUFF 18377 — PORTED 2026-10-24: creature:HasAura is
-- bridged on the handler surface (lua_creature_events.go:388,
-- creatureAuras + activeCreatureAuras keyed by motion), so the
-- kodo-side gate lands in kodoSpellHit below.
-- Ported 2026-11-06 (tick 279): the 18153 arm's mutation half —
-- UpdateEntry(NPC_TAMED_KODO 11627), CombatStop, SetFaction
-- (FRIENDLY), SetSpeedRate(run 0.6f), MoveFollow(PET_FOLLOW_DIST
-- 1.0f, GetFollowAngle() 0 — the kodo never sets a follow angle),
-- RemoveFlag(UNIT_NPC_FLAG_GOSSIP). The 2026-10-24 "MoveFollow is
-- not [bridged]" claim is stale — MoveFollow is bridged
-- (lua_creature_events.go:4650). Deltas: EngagementOver and
-- setActive(true) have no bridge (the kodo is not in combat when
-- the item lands, so CombatStop covers the observable teardown).
-- Ported 2026-11-06 (tick 279): the 18362 (SPELL_KODO_KOMBO_
-- GOSSIP) SpellHit arm — SetFlag gossip, SetHomePosition,
-- MoveIdle, DespawnOrUnsummon(60s). The 2026-10-24 "no flag /
-- movement / despawn bridges" claim is stale — SetFlag, MoveIdle
-- (= StopMoving), DespawnOrUnsummon(ms) are all bridged.
-- Deltas: MotionMaster Clear (no bridge; MoveIdle is the idle
-- twin) and setActive(false) (no bridge).
-- Ported 2026-11-06 (tick 279): OnGossipHello — flag-gated on
-- the gossip flag the 18362 arm sets (without it the client
-- cannot open gossip at all, matching C++ reachability);
-- TalkedToCreature + RemoveAura(18172) + GossipSendMenu all
-- bridged. Dormant until the 18362 spell hits (its only in-tree
-- caster is the unported MoveInLineOfSight leg below).
-- Still unmodeled: MoveInLineOfSight (the Smeed 11596 proximity
-- latch -> SPELL_KODO_KOMBO_GOSSIP 18362 triggered self-cast
-- + smeed Talk(SAY_SMEED_HOME 0)) stays out on trigger grounds, not
-- on Talk grounds: Go ev27 fires only from the aggro-engage branch
-- and Smeed never engages, so the 10.0 proximity latch can never
-- fire (the Talk leg itself IS bridged — obtained-object Talk via
-- FindNearestCreature, the krick/darnavan precedent — but it has no
-- trigger to hang on).
-- In the Go model the event-14 caster is always the player
-- session (no creature-caster spell path), so the C++
-- caster->ToUnit() gate always passes here.

local NPC_AGED_KODO = 4700
local NPC_DYING_KODO = 4701
local NPC_ANCIENT_KODO = 4702
local NPC_TAMED_KODO = 11627

local SPELL_KODO_KOMBO_ITEM = 18153
local SPELL_KODO_KOMBO_PLAYER_BUFF = 18172
local SPELL_KODO_KOMBO_DESPAWN_BUFF = 18377
local SPELL_KODO_KOMBO_GOSSIP = 18362

local FACTION_FRIENDLY = 35
local MOVE_RUN = 1
local PET_FOLLOW_DIST = 1.0 -- PetDefines.h:84
local UNIT_NPC_FLAGS_FIELD = 82 -- blackfathom precedent
local UNIT_NPC_FLAG_GOSSIP = 1

local function kodoSpellHit(_, creature, caster, spellId)
    -- C++ SpellHit arm: SPELL_KODO_KOMBO_GOSSIP (zone_desolace.cpp:
    -- 106-114): set the gossip flag, anchor home here, idle, and
    -- despawn after 60s.
    if spellId == SPELL_KODO_KOMBO_GOSSIP then
        creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
        creature:MoveIdle()
        creature:DespawnOrUnsummon(60000)
        return
    end
    -- C++ SpellHit arm: SPELL_KODO_KOMBO_ITEM.
    if spellId ~= SPELL_KODO_KOMBO_ITEM then
        return
    end
    local entry = creature:GetEntry()
    if entry ~= NPC_AGED_KODO and entry ~= NPC_DYING_KODO
        and entry ~= NPC_ANCIENT_KODO then
        return
    end
    if caster:HasAura(SPELL_KODO_KOMBO_PLAYER_BUFF) then
        return
    end
    -- C++ SpellHit gate (zone_desolace.cpp:85): !(caster 18172
    -- || kodo 18377) — the kodo-side gate lands via
    -- handler-creature HasAura (bridged 2026-10-24).
    if creature:HasAura(SPELL_KODO_KOMBO_DESPAWN_BUFF) then
        return
    end
    caster:AddAura(SPELL_KODO_KOMBO_PLAYER_BUFF)
    creature:CastSpell(creature, SPELL_KODO_KOMBO_DESPAWN_BUFF, true)
    -- C++ mutation half (zone_desolace.cpp:93-101): re-seat as the
    -- tamed kodo, stop combat, go friendly, slow run to 0.6, follow
    -- the caster, drop the gossip flag. EngagementOver + setActive
    -- have no bridge (documented deltas above).
    creature:UpdateEntry(NPC_TAMED_KODO)
    creature:CombatStop()
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetSpeedRate(MOVE_RUN, 0.6)
    creature:MoveFollow(caster, PET_FOLLOW_DIST, 0)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
end

-- C++ OnGossipHello (zone_desolace.cpp:117-127): with the player
-- buff + kodo despawn buff both present, credit the talk and strip
-- the player buff; always send the creature's gossip text.
local function kodoGossipHello(event, player, creature)
    if not creature:HasFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP) then
        return
    end
    if player:HasAura(SPELL_KODO_KOMBO_PLAYER_BUFF)
        and creature:HasAura(SPELL_KODO_KOMBO_DESPAWN_BUFF) then
        player:TalkedToCreature(creature:GetEntry(), creature)
        player:RemoveAura(SPELL_KODO_KOMBO_PLAYER_BUFF)
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

RegisterCreatureEvent(NPC_AGED_KODO, 14, kodoSpellHit)
RegisterCreatureEvent(NPC_DYING_KODO, 14, kodoSpellHit)
RegisterCreatureEvent(NPC_ANCIENT_KODO, 14, kodoSpellHit)

-- The tamed kodo keeps the same AI object across UpdateEntry, so the
-- C++ gossip hello still answers on entry 11627.
RegisterCreatureGossipEvent(NPC_AGED_KODO, 1, kodoGossipHello)
RegisterCreatureGossipEvent(NPC_DYING_KODO, 1, kodoGossipHello)
RegisterCreatureGossipEvent(NPC_ANCIENT_KODO, 1, kodoGossipHello)
RegisterCreatureGossipEvent(NPC_TAMED_KODO, 1, kodoGossipHello)
