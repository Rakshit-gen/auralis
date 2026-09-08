from __future__ import annotations

from auralis_common.settings import BaseServiceSettings


class Settings(BaseServiceSettings):
    recommendation_database_url: str
    recommendation_http_addr: str = "0.0.0.0:8086"
    recommendation_run_consumer: bool = True

    # Ranking model. RECO_WEIGHTS is a JSON object overriding named feature
    # weights; see auralis_reco/ranking.py for the defaults.
    reco_weights: str = ""
    reco_feed_size: int = 30
    reco_diversity: float = 0.35

    @property
    def port(self) -> int:
        return int(self.recommendation_http_addr.rsplit(":", 1)[-1])
