-- Temple of Ahn'Qiraj: Anubisath Sentinel — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/mob_anubisath_sentinel.cpp
-- (npc_anubisath_sentinelAI : public ScriptedAI via GetAQ40AI;
-- registered in AddSC_npc_anubisath_sentinel()).
-- Entry: 15264 (the buddy-grid search uses the sentinel's own entry).
-- Ported: on EVERY enter combat the sentinel picks one of the 9
-- sentinel ability buffs at random (C++ JustEngagedWith ->
-- GetOtherSentinels -> selectAbility(pickAbilityRandom); Reset()
-- re-arms gatherOthersWhenAggro, so evade -> next engage re-picks;
-- uniform 1-9 pick, matching C++ rand32()%9 on a fresh dedup set)
-- and self-casts it (GainSentinelAbility -> AddAura(id, me)).
-- Zero Talk() in C++.
-- Documented-only: the whole buddy machinery (AddSentinelsNear grid
-- scan, GiveBuddyMyList/SendMyListToBuddies/CallBuddiesToAttack,
-- gatherOthersWhenAggro, cross-AI selectAbility dedup, JustDied
-- 50% heal + ability transfer, Reset buddy respawn, DoZoneInCombat) —
-- no ObjectAccessor / cross-AI / GetCreatureListWithEntryInGrid /
-- AttackStart bridge exists in Lua. The Shadow Storm SDComment
-- (storm should only hit targets outside melee range) is an aura-level
-- concern, not scripted here.
local ENTRY = 15264

local BUFFS = {
    2147,  -- SPELL_MENDING_BUFF
    21737, -- SPELL_KNOCK_BUFF
    812,   -- SPELL_MANAB_BUFF
    13022, -- SPELL_REFLECTAF_BUFF
    19595, -- SPELL_REFLECTSFr_BUFF
    25777, -- SPELL_THORNS_BUFF
    2834,  -- SPELL_THUNDER_BUFF
    9347,  -- SPELL_MSTRIKE_BUFF
    2148,  -- SPELL_STORM_BUFF
}

local function onEnterCombat(event, creature, target)
    -- C++ re-picks a random ability on EVERY engage (see header).
    local ability = BUFFS[math.random(#BUFFS)]
    creature:CastSpell(creature, ability)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
